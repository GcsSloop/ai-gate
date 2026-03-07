package policy

type Definition struct {
	Name                    string              `json:"name"`
	CandidateOrder          []string            `json:"candidate_order"`
	MinimumBalanceThreshold float64             `json:"minimum_balance_threshold"`
	MinimumQuotaThreshold   float64             `json:"minimum_quota_threshold"`
	TokenBudgetFactor       float64             `json:"token_budget_factor"`
	ModelPoolRules          map[string][]string `json:"model_pool_rules"`
}
