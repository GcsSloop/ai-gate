package routing

// CanAttemptCandidate reports whether a candidate may be attempted by automatic
// routing. Locked accounts remain usable only when manually selected as active.
func CanAttemptCandidate(candidate Candidate) bool {
	return !candidate.Account.IsLocked || candidate.Account.IsActive
}
