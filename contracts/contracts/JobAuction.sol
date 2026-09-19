// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

interface IProviderRegistry {
    struct Provider {
        address wallet;
        string  endpoint;
        uint256 pricePerHour;
        uint256 stakedAmount;
        uint256 slashCount;
        uint256 jobsCompleted;
        bool    active;
    }
    function getActiveProviders() external view returns (Provider[] memory);
}

interface IDeploymentEscrow {
    function startSession(bytes32 sessionId, address provider, uint256 ratePerSecond) external payable;
}

/// @title JobAuction
/// @notice On-chain request-for-quote auction for MetaMax deployment jobs.
///
///         Flow:
///           1. User calls postJob() with MON deposit. A 30-second bid window opens.
///           2. Active staked providers call submitBid() with their price.
///           3. After the window, anyone calls closeAuction() — lowest bid wins.
///              MON is forwarded to DeploymentEscrow.startSession().
///           4. If no bids arrive, MetaMax's fallback node is used.
///           5. User calls cancelJob() to reclaim deposit if no bids and window closed.
contract JobAuction is Ownable, ReentrancyGuard {
    // ─── Constants ────────────────────────────────────────────────────────────

    uint256 public constant BID_WINDOW = 30 seconds;
    uint256 public constant MIN_STAKE  = 0.01 ether;

    // ─── Types ────────────────────────────────────────────────────────────────

    enum JobStatus { Open, Awarded, Cancelled }

    struct JobRequest {
        address   user;
        uint256   depositedAt;
        uint256   maxPricePerHour;
        uint256   ramMb;
        uint256   cpuCores;
        uint256   durationSeconds;
        JobStatus status;
        address   winningProvider;
        uint256   winningRatePerSecond;
    }

    struct Bid {
        address provider;
        uint256 pricePerHour;
        uint256 submittedAt;
    }

    // ─── State ────────────────────────────────────────────────────────────────

    IProviderRegistry public immutable registry;
    IDeploymentEscrow public immutable escrow;

    address public fallbackProvider;

    mapping(bytes32 => JobRequest)              public jobs;
    mapping(bytes32 => Bid[])                   public bids;
    mapping(bytes32 => mapping(address => bool)) public hasBid;

    // ─── Events ───────────────────────────────────────────────────────────────

    event JobPosted(bytes32 indexed jobId, address indexed user, uint256 maxPricePerHour, uint256 ramMb, uint256 cpuCores, uint256 durationSeconds, uint256 deposit, uint256 bidDeadline);
    event BidSubmitted(bytes32 indexed jobId, address indexed provider, uint256 pricePerHour);
    event JobAwarded(bytes32 indexed jobId, address indexed winner, uint256 pricePerHour, uint256 ratePerSecond);
    event JobAwardedToFallback(bytes32 indexed jobId, address indexed fallbackNode, uint256 ratePerSecond);
    event JobCancelled(bytes32 indexed jobId, address indexed user, uint256 refunded);
    event FallbackProviderUpdated(address indexed newFallback);

    // ─── Errors ───────────────────────────────────────────────────────────────

    error JobAlreadyExists();
    error JobNotFound();
    error WrongStatus();
    error BidWindowOpen();
    error BidWindowClosed();
    error DuplicateBid();
    error BidAboveCeiling();
    error NotJobOwner();
    error InsufficientStake();
    error ZeroDeposit();
    error ZeroDuration();
    error NoFallbackProvider();

    // ─── Constructor ──────────────────────────────────────────────────────────

    constructor(
        address initialOwner,
        address _registry,
        address _escrow,
        address _fallbackProvider
    ) Ownable(initialOwner) {
        registry         = IProviderRegistry(_registry);
        escrow           = IDeploymentEscrow(_escrow);
        fallbackProvider = _fallbackProvider;
    }

    // ─── User actions ─────────────────────────────────────────────────────────

    /// @notice Post a deployment job and open the bid window.
    /// @param jobId            keccak256 of the session identifier (assigned by backend)
    /// @param maxPricePerHour  Maximum wei/hr the user is willing to pay
    /// @param ramMb            Required RAM in megabytes
    /// @param cpuCores         Required CPU cores
    /// @param durationSeconds  Estimated session duration
    function postJob(
        bytes32 jobId,
        uint256 maxPricePerHour,
        uint256 ramMb,
        uint256 cpuCores,
        uint256 durationSeconds
    ) external payable nonReentrant {
        if (msg.value == 0) revert ZeroDeposit();
        if (durationSeconds == 0) revert ZeroDuration();
        if (jobs[jobId].user != address(0)) revert JobAlreadyExists();

        jobs[jobId] = JobRequest({
            user:                 msg.sender,
            depositedAt:          block.timestamp,
            maxPricePerHour:      maxPricePerHour,
            ramMb:                ramMb,
            cpuCores:             cpuCores,
            durationSeconds:      durationSeconds,
            status:               JobStatus.Open,
            winningProvider:      address(0),
            winningRatePerSecond: 0
        });

        emit JobPosted(jobId, msg.sender, maxPricePerHour, ramMb, cpuCores, durationSeconds, msg.value, block.timestamp + BID_WINDOW);
    }

    /// @notice Cancel an open job. Can cancel any time if no bids; otherwise only after window closes.
    function cancelJob(bytes32 jobId) external nonReentrant {
        JobRequest storage j = jobs[jobId];
        if (j.user == address(0)) revert JobNotFound();
        if (j.status != JobStatus.Open) revert WrongStatus();
        if (msg.sender != j.user) revert NotJobOwner();

        bool windowClosed = block.timestamp > j.depositedAt + BID_WINDOW;
        if (!windowClosed && bids[jobId].length > 0) revert BidWindowOpen();

        j.status = JobStatus.Cancelled;
        uint256 refundAmount = address(this).balance;
        emit JobCancelled(jobId, j.user, refundAmount);
        (bool ok, ) = j.user.call{value: refundAmount}("");
        require(ok, "Refund failed");
    }

    // ─── Provider actions ─────────────────────────────────────────────────────

    /// @notice Submit a bid. Provider must be active and staked in ProviderRegistry.
    /// @param jobId         The job to bid on
    /// @param pricePerHour  Wei per hour the provider is offering
    function submitBid(bytes32 jobId, uint256 pricePerHour) external {
        JobRequest storage j = jobs[jobId];
        if (j.user == address(0)) revert JobNotFound();
        if (j.status != JobStatus.Open) revert WrongStatus();
        if (block.timestamp > j.depositedAt + BID_WINDOW) revert BidWindowClosed();
        if (hasBid[jobId][msg.sender]) revert DuplicateBid();
        if (pricePerHour > j.maxPricePerHour) revert BidAboveCeiling();
        _requireSufficientStake(msg.sender);

        hasBid[jobId][msg.sender] = true;
        bids[jobId].push(Bid({
            provider:     msg.sender,
            pricePerHour: pricePerHour,
            submittedAt:  block.timestamp
        }));

        emit BidSubmitted(jobId, msg.sender, pricePerHour);
    }

    // ─── Settlement ───────────────────────────────────────────────────────────

    /// @notice Close the auction. Selects lowest bidder and forwards MON to escrow.
    ///         Falls back to MetaMax node if no bids. Callable by anyone after window closes.
    function closeAuction(bytes32 jobId) external nonReentrant {
        JobRequest storage j = jobs[jobId];
        if (j.user == address(0)) revert JobNotFound();
        if (j.status != JobStatus.Open) revert WrongStatus();
        if (block.timestamp <= j.depositedAt + BID_WINDOW) revert BidWindowOpen();

        uint256 deposit = address(this).balance;

        // Fallback: no bids → use MetaMax's own node
        if (bids[jobId].length == 0) {
            if (fallbackProvider == address(0)) revert NoFallbackProvider();
            uint256 fallbackRate = j.maxPricePerHour / 3600;
            if (fallbackRate == 0) fallbackRate = 1;
            j.status               = JobStatus.Awarded;
            j.winningProvider      = fallbackProvider;
            j.winningRatePerSecond = fallbackRate;
            emit JobAwardedToFallback(jobId, fallbackProvider, fallbackRate);
            escrow.startSession{value: deposit}(jobId, fallbackProvider, fallbackRate);
            return;
        }

        // Pick lowest bid; tie-break by earliest submission
        Bid memory winner = bids[jobId][0];
        for (uint256 i = 1; i < bids[jobId].length; i++) {
            Bid memory b = bids[jobId][i];
            if (b.pricePerHour < winner.pricePerHour ||
                (b.pricePerHour == winner.pricePerHour && b.submittedAt < winner.submittedAt)) {
                winner = b;
            }
        }

        uint256 ratePerSecond = winner.pricePerHour / 3600;
        if (ratePerSecond == 0) ratePerSecond = 1;

        j.status               = JobStatus.Awarded;
        j.winningProvider      = winner.provider;
        j.winningRatePerSecond = ratePerSecond;

        emit JobAwarded(jobId, winner.provider, winner.pricePerHour, ratePerSecond);
        escrow.startSession{value: deposit}(jobId, winner.provider, ratePerSecond);
    }

    // ─── Admin ────────────────────────────────────────────────────────────────

    function setFallbackProvider(address newFallback) external onlyOwner {
        fallbackProvider = newFallback;
        emit FallbackProviderUpdated(newFallback);
    }

    // ─── View helpers ─────────────────────────────────────────────────────────

    function getBids(bytes32 jobId) external view returns (Bid[] memory) {
        return bids[jobId];
    }

    function lowestBid(bytes32 jobId) external view returns (address provider, uint256 pricePerHour) {
        Bid[] storage b = bids[jobId];
        if (b.length == 0) return (address(0), 0);
        Bid memory best = b[0];
        for (uint256 i = 1; i < b.length; i++) {
            if (b[i].pricePerHour < best.pricePerHour) best = b[i];
        }
        return (best.provider, best.pricePerHour);
    }

    function isBidWindowOpen(bytes32 jobId) external view returns (bool) {
        JobRequest storage j = jobs[jobId];
        if (j.user == address(0)) return false;
        return j.status == JobStatus.Open && block.timestamp <= j.depositedAt + BID_WINDOW;
    }

    // ─── Internal ─────────────────────────────────────────────────────────────

    function _requireSufficientStake(address provider) internal view {
        IProviderRegistry.Provider[] memory active = registry.getActiveProviders();
        for (uint256 i = 0; i < active.length; i++) {
            if (active[i].wallet == provider) {
                if (active[i].stakedAmount < MIN_STAKE) revert InsufficientStake();
                return;
            }
        }
        revert InsufficientStake();
    }
}
