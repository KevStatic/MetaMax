// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/// @title ProviderRegistry
/// @notice On-chain marketplace for MetaMax compute providers.
///         Providers stake MON to join, are ranked by price and reputation,
///         and can be slashed if they misbehave.
contract ProviderRegistry is Ownable, ReentrancyGuard {
    // ─── Constants ────────────────────────────────────────────────────────────

    /// @notice Minimum stake required to become an active provider (0.01 MON)
    uint256 public constant MIN_STAKE = 0.01 ether;

    // ─── Types ────────────────────────────────────────────────────────────────

    struct Provider {
        address wallet;
        string  endpoint;       // HTTPS URL of the provider node API
        uint256 pricePerHour;   // Price in wei per container-hour
        uint256 stakedAmount;   // Current stake held in this contract
        uint256 slashCount;     // Number of times slashed
        uint256 jobsCompleted;  // Successfully completed deploy sessions
        bool    active;         // Whether the provider is accepting jobs
    }

    // ─── State ────────────────────────────────────────────────────────────────

    mapping(address => Provider) public providers;
    address[] public providerList;

    /// @notice Address authorised to submit slash evidence (MetaMax dispute resolver)
    address public slashAuthority;

    // ─── Events ───────────────────────────────────────────────────────────────

    event ProviderRegistered(address indexed wallet, string endpoint, uint256 pricePerHour, uint256 stakedAmount);
    event ProviderUpdated(address indexed wallet, string endpoint, uint256 pricePerHour);
    event ProviderDeactivated(address indexed wallet);
    event ProviderReactivated(address indexed wallet);
    event Staked(address indexed wallet, uint256 amount, uint256 totalStake);
    event Unstaked(address indexed wallet, uint256 amount, uint256 remainingStake);
    event Slashed(address indexed wallet, uint256 amount, bytes32 evidence);
    event JobCompleted(address indexed wallet, uint256 totalJobs);
    event SlashAuthorityUpdated(address indexed newAuthority);

    // ─── Errors ───────────────────────────────────────────────────────────────

    error AlreadyRegistered();
    error NotRegistered();
    error InsufficientStake();
    error StakeBelowMinAfterUnstake();
    error Unauthorised();
    error ZeroAmount();
    error EmptyEndpoint();

    // ─── Constructor ──────────────────────────────────────────────────────────

    constructor(address initialOwner, address _slashAuthority) Ownable(initialOwner) {
        slashAuthority = _slashAuthority;
    }

    // ─── Registration ─────────────────────────────────────────────────────────

    /// @notice Register as a compute provider. Must send >= MIN_STAKE as msg.value.
    /// @param endpoint      HTTPS URL of the provider's node API
    /// @param pricePerHour  Price in wei the provider charges per container-hour
    function register(string calldata endpoint, uint256 pricePerHour) external payable nonReentrant {
        if (providers[msg.sender].wallet != address(0)) revert AlreadyRegistered();
        if (msg.value < MIN_STAKE) revert InsufficientStake();
        if (bytes(endpoint).length == 0) revert EmptyEndpoint();

        providers[msg.sender] = Provider({
            wallet:        msg.sender,
            endpoint:      endpoint,
            pricePerHour:  pricePerHour,
            stakedAmount:  msg.value,
            slashCount:    0,
            jobsCompleted: 0,
            active:        true
        });

        providerList.push(msg.sender);
        emit ProviderRegistered(msg.sender, endpoint, pricePerHour, msg.value);
    }

    /// @notice Update endpoint or price. Only the provider itself can call this.
    function update(string calldata endpoint, uint256 pricePerHour) external {
        if (providers[msg.sender].wallet == address(0)) revert NotRegistered();
        if (bytes(endpoint).length == 0) revert EmptyEndpoint();
        providers[msg.sender].endpoint     = endpoint;
        providers[msg.sender].pricePerHour = pricePerHour;
        emit ProviderUpdated(msg.sender, endpoint, pricePerHour);
    }

    // ─── Staking ──────────────────────────────────────────────────────────────

    /// @notice Top up stake on an existing registration.
    function stake() external payable nonReentrant {
        if (providers[msg.sender].wallet == address(0)) revert NotRegistered();
        if (msg.value == 0) revert ZeroAmount();
        providers[msg.sender].stakedAmount += msg.value;
        if (!providers[msg.sender].active && providers[msg.sender].stakedAmount >= MIN_STAKE) {
            providers[msg.sender].active = true;
            emit ProviderReactivated(msg.sender);
        }
        emit Staked(msg.sender, msg.value, providers[msg.sender].stakedAmount);
    }

    /// @notice Withdraw part of the stake. If remaining stake falls below MIN_STAKE,
    ///         provider is deactivated.
    function unstake(uint256 amount) external nonReentrant {
        if (providers[msg.sender].wallet == address(0)) revert NotRegistered();
        if (amount == 0) revert ZeroAmount();
        if (amount > providers[msg.sender].stakedAmount) revert InsufficientStake();

        uint256 remaining = providers[msg.sender].stakedAmount - amount;
        providers[msg.sender].stakedAmount = remaining;

        if (remaining < MIN_STAKE && providers[msg.sender].active) {
            providers[msg.sender].active = false;
            emit ProviderDeactivated(msg.sender);
        }

        emit Unstaked(msg.sender, amount, remaining);
        (bool ok, ) = msg.sender.call{value: amount}("");
        require(ok, "MON transfer failed");
    }

    // ─── Activation ───────────────────────────────────────────────────────────

    function deactivate() external {
        if (providers[msg.sender].wallet == address(0)) revert NotRegistered();
        providers[msg.sender].active = false;
        emit ProviderDeactivated(msg.sender);
    }

    function reactivate() external {
        if (providers[msg.sender].wallet == address(0)) revert NotRegistered();
        if (providers[msg.sender].stakedAmount < MIN_STAKE) revert InsufficientStake();
        providers[msg.sender].active = true;
        emit ProviderReactivated(msg.sender);
    }

    // ─── Slashing ─────────────────────────────────────────────────────────────

    /// @notice Slash 50% of a misbehaving provider's stake. Slashed funds go to owner (treasury).
    /// @param providerWallet  Provider to slash
    /// @param evidence        keccak256 hash of evidence (e.g. IPFS CID)
    function slash(address providerWallet, bytes32 evidence) external nonReentrant {
        if (msg.sender != slashAuthority && msg.sender != owner()) revert Unauthorised();
        Provider storage p = providers[providerWallet];
        if (p.wallet == address(0)) revert NotRegistered();

        uint256 slashAmount = p.stakedAmount / 2;
        p.stakedAmount -= slashAmount;
        p.slashCount   += 1;

        if (p.stakedAmount < MIN_STAKE) {
            p.active = false;
            emit ProviderDeactivated(providerWallet);
        }

        emit Slashed(providerWallet, slashAmount, evidence);
        (bool ok, ) = owner().call{value: slashAmount}("");
        require(ok, "Slash transfer failed");
    }

    // ─── Job tracking ─────────────────────────────────────────────────────────

    /// @notice Increment a provider's completed-job counter.
    ///         Only callable by slashAuthority or owner (the MetaMax backend).
    function recordJobCompleted(address providerWallet) external {
        if (msg.sender != slashAuthority && msg.sender != owner()) revert Unauthorised();
        if (providers[providerWallet].wallet == address(0)) revert NotRegistered();
        providers[providerWallet].jobsCompleted += 1;
        emit JobCompleted(providerWallet, providers[providerWallet].jobsCompleted);
    }

    // ─── Admin ────────────────────────────────────────────────────────────────

    function setSlashAuthority(address newAuthority) external onlyOwner {
        slashAuthority = newAuthority;
        emit SlashAuthorityUpdated(newAuthority);
    }

    // ─── View helpers ─────────────────────────────────────────────────────────

    /// @notice Return all currently active providers.
    function getActiveProviders() external view returns (Provider[] memory) {
        uint256 count = 0;
        for (uint256 i = 0; i < providerList.length; i++) {
            if (providers[providerList[i]].active) count++;
        }
        Provider[] memory active = new Provider[](count);
        uint256 idx = 0;
        for (uint256 i = 0; i < providerList.length; i++) {
            if (providers[providerList[i]].active) {
                active[idx++] = providers[providerList[i]];
            }
        }
        return active;
    }

    /// @notice Return all registered providers (active and inactive).
    function getAllProviders() external view returns (Provider[] memory) {
        Provider[] memory all = new Provider[](providerList.length);
        for (uint256 i = 0; i < providerList.length; i++) {
            all[i] = providers[providerList[i]];
        }
        return all;
    }

    /// @notice Total number of registered providers.
    function providerCount() external view returns (uint256) {
        return providerList.length;
    }
}
