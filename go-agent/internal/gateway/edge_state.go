package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

type edgeState struct {
	mu sync.RWMutex

	finetuneSeq  int
	finetuneJobs []map[string]any

	enclaveProofCount int
	lastEnclaveProof  map[string]any
}

func newEdgeState() *edgeState {
	return &edgeState{
		finetuneJobs:      make([]map[string]any, 0, 128),
		lastEnclaveProof:  map[string]any{},
		enclaveProofCount: 0,
	}
}

func (e *edgeState) addFinetuneJob(params map[string]any, status string) map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.finetuneSeq++
	job := map[string]any{
		"id":        fmt.Sprintf("ft-%04d", e.finetuneSeq),
		"status":    status,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
		"params":    cloneMap(params),
	}
	e.finetuneJobs = append(e.finetuneJobs, job)
	if len(e.finetuneJobs) > 256 {
		e.finetuneJobs = append([]map[string]any(nil), e.finetuneJobs[len(e.finetuneJobs)-256:]...)
	}
	return cloneMap(job)
}

func (e *edgeState) listFinetuneJobs(limit int) []map[string]any {
	if limit <= 0 {
		limit = 25
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.finetuneJobs) == 0 {
		return []map[string]any{
			{
				"id":      "ft-0000",
				"status":  "idle",
				"message": "no finetune jobs recorded",
			},
		}
	}
	start := 0
	if len(e.finetuneJobs) > limit {
		start = len(e.finetuneJobs) - limit
	}
	out := make([]map[string]any, 0, len(e.finetuneJobs)-start)
	for _, job := range e.finetuneJobs[start:] {
		out = append(out, cloneMap(job))
	}
	return out
}

func (e *edgeState) issueEnclaveProof(challenge string) map[string]any {
	challenge = strings.TrimSpace(challenge)
	if challenge == "" {
		challenge = "default-challenge"
	}
	now := time.Now().UTC()
	hashInput := []byte(challenge + "|" + now.Format(time.RFC3339Nano))
	digest := sha256.Sum256(hashInput)
	proof := map[string]any{
		"challenge": challenge,
		"proof":     "enclave-proof-" + hex.EncodeToString(digest[:12]),
		"issuedAt":  now.Format(time.RFC3339),
	}

	e.mu.Lock()
	e.enclaveProofCount++
	e.lastEnclaveProof = cloneMap(proof)
	e.mu.Unlock()
	return proof
}

func (e *edgeState) recordEnclaveProof(challenge string, proofValue string, issuedAt string) map[string]any {
	challenge = strings.TrimSpace(challenge)
	if challenge == "" {
		challenge = "default-challenge"
	}
	if strings.TrimSpace(issuedAt) == "" {
		issuedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(proofValue) == "" {
		hashInput := []byte(challenge + "|" + issuedAt)
		digest := sha256.Sum256(hashInput)
		proofValue = "enclave-proof-" + hex.EncodeToString(digest[:12])
	}
	proof := map[string]any{
		"challenge": challenge,
		"proof":     proofValue,
		"issuedAt":  issuedAt,
	}

	e.mu.Lock()
	e.enclaveProofCount++
	e.lastEnclaveProof = cloneMap(proof)
	e.mu.Unlock()
	return proof
}

func (e *edgeState) enclaveStatus() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := map[string]any{
		"enabled":       true,
		"attestation":   "ready",
		"proofCount":    e.enclaveProofCount,
		"lastProofAt":   "",
		"lastChallenge": "",
	}
	if len(e.lastEnclaveProof) > 0 {
		out["lastProofAt"] = toString(e.lastEnclaveProof["issuedAt"], "")
		out["lastChallenge"] = toString(e.lastEnclaveProof["challenge"], "")
		out["lastProof"] = cloneMap(e.lastEnclaveProof)
	} else {
		out["lastProof"] = map[string]any{}
	}
	return out
}
