package domain

const (
	MaxAmountCents = 5000000
	VelocityPerMinLimit = 10
)

type RuleResult struct {
	Verdict Verdict
	Reason 	string
	Final 	bool
}

var Blocklist = map[string]bool {
	"fp_stolen_card_001": true,
	"fp_stolen_card_002": true, 
}

func ApplyFastRules(a Attempt) RuleResult {
	if Blocklist[a.CardFingerPrint] {
		return RuleResult{Verdict: VerdictDeny, Reason: "card_blocklisted", Final: true}
	}
	if a.AmountCents > MaxAmountCents {
		return RuleResult{Verdict: VerdictDeny, Reason: "amount_above_hard_limit", Final: true}
	}
	if a.VelocityLastMin > VelocityPerMinLimit {
		return RuleResult{Verdict: VerdictDeny, Reason: "velocity_limit_exceeded", Final: true}
	}
	return RuleResult{Verdict: VerdictApprove, Reason: "fast_rules_passed", Final: false}

} 