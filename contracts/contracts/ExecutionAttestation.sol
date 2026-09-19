// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

import "@openzeppelin/contracts/access/Ownable.sol";

/// @notice Minimal native Monad attestation registry for MetaMax execution proofs.
/// The backend attester commits a session identifier and Merkle root; raw workload
/// data never reaches the chain. Anyone can independently verify the committed root.
contract ExecutionAttestation is Ownable {
    struct Attestation {
        bytes32 teamId;
        bytes32 merkleRoot;
        address attester;
        uint64 timestamp;
        bool revoked;
    }

    mapping(bytes32 => Attestation) public attestations;

    event ExecutionAttested(
        bytes32 indexed sessionHash,
        string sessionId,
        bytes32 indexed teamId,
        bytes32 merkleRoot,
        address indexed attester
    );
    event AttestationRevoked(bytes32 indexed sessionHash, string sessionId);

    error AttestationExists();
    error AttestationMissing();

    constructor(address initialOwner) Ownable(initialOwner) {}

    function attest(string calldata sessionId, bytes32 teamId, bytes32 merkleRoot) external {
        bytes32 sessionHash = keccak256(bytes(sessionId));
        if (attestations[sessionHash].timestamp != 0) revert AttestationExists();
        attestations[sessionHash] = Attestation(teamId, merkleRoot, msg.sender, uint64(block.timestamp), false);
        emit ExecutionAttested(sessionHash, sessionId, teamId, merkleRoot, msg.sender);
    }

    function revoke(string calldata sessionId) external onlyOwner {
        bytes32 sessionHash = keccak256(bytes(sessionId));
        if (attestations[sessionHash].timestamp == 0) revert AttestationMissing();
        attestations[sessionHash].revoked = true;
        emit AttestationRevoked(sessionHash, sessionId);
    }

    function isValid(string calldata sessionId, bytes32 merkleRoot) external view returns (bool) {
        Attestation memory a = attestations[keccak256(bytes(sessionId))];
        return a.timestamp != 0 && !a.revoked && a.merkleRoot == merkleRoot;
    }
}
