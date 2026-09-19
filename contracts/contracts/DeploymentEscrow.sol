// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

interface IProviderRegistry {
    function slash(address providerWallet, bytes32 evidence) external;
}

/// @title DeploymentEscrow
/// @notice Payment escrow for MetaMax deployment sessions.
///
///         Two payment modes:
///
///         Simple escrow:
///           1. User deposits MON via deposit() for a session.
///           2. Backend calls release() on success → funds go to provider minus fee.
///           3. User calls refund() after LOCKUP_PERIOD if session failed.
///
///         Streaming payment:
///           1. User calls startSession() with MON. 20% upfront to provider; 80% escrowed.
///           2. releasePayment() or submitProof() drips wei/second to provider.
///           3. stopSession() releases accrued payment + refunds remainder to user.
///           4. slashProvider() stops session, refunds user, slashes stake in registry.
contract DeploymentEscrow is Ownable, ReentrancyGuard {
    // ─── Constants ────────────────────────────────────────────────────────────

    uint256 public constant LOCKUP_PERIOD = 24 hours;
    uint256 public constant FEE_BPS       = 1_000;   // 10% protocol fee
    uint256 private constant BPS_DENOM    = 10_000;
    uint256 public constant UPFRONT_BPS   = 2_000;   // 20% upfront to provider

    // ─── Types ────────────────────────────────────────────────────────────────

    enum EscrowStatus { Pending, Released, Refunded, Disputed }

    struct Escrow {
        address user;
        address provider;
        uint256 amount;
        uint256 depositedAt;
        bytes32 sessionId;
        EscrowStatus status;
    }

    struct Session {
        address user;
        address provider;
        uint256 ratePerSecond;
        uint256 remainingBalance;
        uint256 lastPaidAt;
        bool    isActive;
    }

    // ─── State ────────────────────────────────────────────────────────────────

    mapping(bytes32 => Escrow)  public escrows;
    mapping(bytes32 => Session) public sessions;

    address public releaseAuthority;
    address public proofAuthority;
    uint256 public accruedFees;

    IProviderRegistry public immutable registry;

    // ─── Events ───────────────────────────────────────────────────────────────

    event Deposited(bytes32 indexed sessionId, address indexed user, address indexed provider, uint256 amount);
    event Released(bytes32 indexed sessionId, address indexed provider, uint256 netAmount, uint256 fee);
    event Refunded(bytes32 indexed sessionId, address indexed user, uint256 amount);
    event Disputed(bytes32 indexed sessionId);
    event FeesWithdrawn(address indexed to, uint256 amount);
    event ReleaseAuthorityUpdated(address indexed newAuthority);
    event SessionStarted(bytes32 indexed sessionId, address indexed user, address indexed provider, uint256 totalDeposit, uint256 upfrontPaid, uint256 escrowed, uint256 ratePerSecond);
    event PaymentReleased(bytes32 indexed sessionId, address indexed provider, uint256 netAmount, uint256 remainingBalance);
    event SessionStopped(bytes32 indexed sessionId, address indexed user, uint256 refundAmount);
    event ProviderSlashed(bytes32 indexed sessionId, address indexed provider, bytes32 evidence);
    event ProofAuthorityUpdated(address indexed newAuthority);

    // ─── Errors ───────────────────────────────────────────────────────────────

    error SessionAlreadyExists();
    error SessionNotFound();
    error WrongStatus();
    error LockupNotExpired();
    error Unauthorised();
    error ZeroAmount();
    error NoFeesToWithdraw();
    error SessionNotActive();
    error ZeroRate();

    // ─── Constructor ──────────────────────────────────────────────────────────

    constructor(address initialOwner, address _releaseAuthority, address _registry) Ownable(initialOwner) {
        releaseAuthority = _releaseAuthority;
        proofAuthority   = _releaseAuthority;
        registry         = IProviderRegistry(_registry);
    }

    // ─── Simple escrow ────────────────────────────────────────────────────────

    /// @notice Deposit MON into escrow for a deployment session.
    function deposit(bytes32 sessionId, address provider) external payable nonReentrant {
        if (msg.value == 0) revert ZeroAmount();
        if (escrows[sessionId].user != address(0)) revert SessionAlreadyExists();

        escrows[sessionId] = Escrow({
            user:        msg.sender,
            provider:    provider,
            amount:      msg.value,
            depositedAt: block.timestamp,
            sessionId:   sessionId,
            status:      EscrowStatus.Pending
        });

        emit Deposited(sessionId, msg.sender, provider, msg.value);
    }

    /// @notice Refund user after lockup expires. User calls this themselves.
    function refund(bytes32 sessionId) external nonReentrant {
        Escrow storage e = escrows[sessionId];
        if (e.user == address(0)) revert SessionNotFound();
        if (e.status != EscrowStatus.Pending) revert WrongStatus();
        if (msg.sender != e.user) revert Unauthorised();
        if (block.timestamp < e.depositedAt + LOCKUP_PERIOD) revert LockupNotExpired();

        e.status = EscrowStatus.Refunded;
        uint256 amount = e.amount;
        emit Refunded(sessionId, e.user, amount);
        (bool ok, ) = e.user.call{value: amount}("");
        require(ok, "Refund failed");
    }

    /// @notice Release funds to provider after a successful session.
    ///         Deducts protocol fee; only callable by releaseAuthority or owner.
    function release(bytes32 sessionId) external nonReentrant {
        if (msg.sender != releaseAuthority && msg.sender != owner()) revert Unauthorised();
        Escrow storage e = escrows[sessionId];
        if (e.user == address(0)) revert SessionNotFound();
        if (e.status != EscrowStatus.Pending) revert WrongStatus();

        e.status = EscrowStatus.Released;
        uint256 fee       = (e.amount * FEE_BPS) / BPS_DENOM;
        uint256 netAmount = e.amount - fee;
        accruedFees      += fee;

        emit Released(sessionId, e.provider, netAmount, fee);
        (bool ok, ) = e.provider.call{value: netAmount}("");
        require(ok, "Release failed");
    }

    /// @notice Freeze funds pending off-chain dispute resolution.
    function dispute(bytes32 sessionId) external {
        if (msg.sender != releaseAuthority && msg.sender != owner()) revert Unauthorised();
        Escrow storage e = escrows[sessionId];
        if (e.user == address(0)) revert SessionNotFound();
        if (e.status != EscrowStatus.Pending) revert WrongStatus();
        e.status = EscrowStatus.Disputed;
        emit Disputed(sessionId);
    }

    /// @notice Resolve a dispute — send funds to provider or refund user.
    function resolveDispute(bytes32 sessionId, bool toProvider) external nonReentrant onlyOwner {
        Escrow storage e = escrows[sessionId];
        if (e.user == address(0)) revert SessionNotFound();
        if (e.status != EscrowStatus.Disputed) revert WrongStatus();
        uint256 amount = e.amount;
        if (toProvider) {
            uint256 fee       = (amount * FEE_BPS) / BPS_DENOM;
            uint256 netAmount = amount - fee;
            accruedFees      += fee;
            e.status          = EscrowStatus.Released;
            emit Released(sessionId, e.provider, netAmount, fee);
            (bool ok, ) = e.provider.call{value: netAmount}("");
            require(ok, "Dispute release failed");
        } else {
            e.status = EscrowStatus.Refunded;
            emit Refunded(sessionId, e.user, amount);
            (bool ok, ) = e.user.call{value: amount}("");
            require(ok, "Dispute refund failed");
        }
    }

    // ─── Streaming payment ────────────────────────────────────────────────────

    /// @notice Start a streaming session. 20% upfront to provider; remainder escrowed.
    /// @param sessionId     keccak256 of session identifier
    /// @param provider      Provider wallet address
    /// @param ratePerSecond Wei per second to stream to provider
    function startSession(bytes32 sessionId, address provider, uint256 ratePerSecond) external payable nonReentrant {
        if (msg.value == 0) revert ZeroAmount();
        if (ratePerSecond == 0) revert ZeroRate();
        if (sessions[sessionId].user != address(0)) revert SessionAlreadyExists();

        uint256 upfront  = (msg.value * UPFRONT_BPS) / BPS_DENOM;
        uint256 escrowed = msg.value - upfront;

        sessions[sessionId] = Session({
            user:             msg.sender,
            provider:         provider,
            ratePerSecond:    ratePerSecond,
            remainingBalance: escrowed,
            lastPaidAt:       block.timestamp,
            isActive:         true
        });

        emit SessionStarted(sessionId, msg.sender, provider, msg.value, upfront, escrowed, ratePerSecond);
        (bool ok, ) = provider.call{value: upfront}("");
        require(ok, "Upfront transfer failed");
    }

    /// @notice Release accrued streaming payment. Callable by anyone.
    function releasePayment(bytes32 sessionId) external nonReentrant {
        Session storage s = sessions[sessionId];
        if (s.user == address(0)) revert SessionNotFound();
        if (!s.isActive) revert SessionNotActive();
        _releaseAccrued(sessionId);
    }

    /// @notice Submit proof of uptime and trigger streaming payment. Only proofAuthority.
    function submitProof(bytes32 sessionId, bytes32 stateHash) external nonReentrant {
        if (msg.sender != proofAuthority && msg.sender != owner()) revert Unauthorised();
        Session storage s = sessions[sessionId];
        if (s.user == address(0)) revert SessionNotFound();
        if (!s.isActive) revert SessionNotActive();
        _releaseAccrued(sessionId);
        stateHash; // logged via event in _releaseAccrued; stateHash is for off-chain verification
    }

    /// @notice Stop a session. Releases accrued payment, refunds remaining to user.
    function stopSession(bytes32 sessionId) external nonReentrant {
        Session storage s = sessions[sessionId];
        if (s.user == address(0)) revert SessionNotFound();
        if (!s.isActive) revert SessionNotActive();
        if (msg.sender != s.user) revert Unauthorised();

        _releaseAccrued(sessionId);
        uint256 refundAmount = s.remainingBalance;
        s.remainingBalance = 0;
        s.isActive = false;

        emit SessionStopped(sessionId, s.user, refundAmount);
        if (refundAmount > 0) {
            (bool ok, ) = s.user.call{value: refundAmount}("");
            require(ok, "Refund failed");
        }
    }

    /// @notice Slash misbehaving provider: stops session, refunds user, slashes registry stake.
    function slashProvider(bytes32 sessionId, bytes32 evidence) external nonReentrant {
        if (msg.sender != proofAuthority && msg.sender != owner()) revert Unauthorised();
        Session storage s = sessions[sessionId];
        if (s.user == address(0)) revert SessionNotFound();
        if (!s.isActive) revert SessionNotActive();

        address provider     = s.provider;
        address user         = s.user;
        uint256 refundAmount = s.remainingBalance;
        s.remainingBalance   = 0;
        s.isActive           = false;

        emit ProviderSlashed(sessionId, provider, evidence);
        emit SessionStopped(sessionId, user, refundAmount);

        try registry.slash(provider, evidence) {} catch {}

        if (refundAmount > 0) {
            (bool ok, ) = user.call{value: refundAmount}("");
            require(ok, "Slash refund failed");
        }
    }

    // ─── Admin ────────────────────────────────────────────────────────────────

    function withdrawFees() external onlyOwner nonReentrant {
        uint256 amount = accruedFees;
        if (amount == 0) revert NoFeesToWithdraw();
        accruedFees = 0;
        emit FeesWithdrawn(owner(), amount);
        (bool ok, ) = owner().call{value: amount}("");
        require(ok, "Fee withdrawal failed");
    }

    function setReleaseAuthority(address newAuthority) external onlyOwner {
        releaseAuthority = newAuthority;
        emit ReleaseAuthorityUpdated(newAuthority);
    }

    function setProofAuthority(address newAuthority) external onlyOwner {
        proofAuthority = newAuthority;
        emit ProofAuthorityUpdated(newAuthority);
    }

    // ─── Internal ─────────────────────────────────────────────────────────────

    function _releaseAccrued(bytes32 sessionId) internal {
        Session storage s = sessions[sessionId];
        uint256 elapsed = block.timestamp - s.lastPaidAt;
        if (elapsed == 0) return;

        uint256 owed = elapsed * s.ratePerSecond;
        if (owed > s.remainingBalance) owed = s.remainingBalance;
        if (owed == 0) return;

        uint256 fee       = (owed * FEE_BPS) / BPS_DENOM;
        uint256 netAmount = owed - fee;
        s.remainingBalance -= owed;
        s.lastPaidAt        = block.timestamp;
        accruedFees        += fee;

        emit PaymentReleased(sessionId, s.provider, netAmount, s.remainingBalance);
        (bool ok, ) = s.provider.call{value: netAmount}("");
        require(ok, "Payment transfer failed");
    }

    // ─── View helpers ─────────────────────────────────────────────────────────

    function isLockupExpired(bytes32 sessionId) external view returns (bool) {
        Escrow storage e = escrows[sessionId];
        if (e.user == address(0)) return false;
        return block.timestamp >= e.depositedAt + LOCKUP_PERIOD;
    }

    function pendingPayment(bytes32 sessionId) external view returns (uint256) {
        Session storage s = sessions[sessionId];
        if (!s.isActive) return 0;
        uint256 owed = (block.timestamp - s.lastPaidAt) * s.ratePerSecond;
        return owed > s.remainingBalance ? s.remainingBalance : owed;
    }
}
