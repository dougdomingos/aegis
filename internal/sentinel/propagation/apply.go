package propagation

import (
	"strings"

	"dougdomingos.com/aegis/internal/common/propagation"
	"dougdomingos.com/aegis/internal/common/rules"
	"dougdomingos.com/aegis/internal/sentinel/daemon"
)

// apply validates one received message against the session and the stored
// state, persists the accepted state atomically, and returns the acknowledgement
// to send back. On any rejection the provided state is returned untouched and
// never persisted.
func (runner *Runner) apply(session *daemon.Session, state State, message *propagation.Message) (State, propagation.Ack) {
	nack := func(reason string) (State, propagation.Ack) {
		return state, propagation.Ack{
			Sequence: message.Sequence,
			Status:   propagation.AckNacked,
			Reason:   reason,
		}
	}

	if message.Epoch == "" {
		return nack("empty epoch")
	}

	if message.Sequence == 0 {
		return nack("empty sequence")
	}

	if message.MachineID != session.MachineID {
		return nack("machine mismatch")
	}

	// An epoch change means the Orchestrator restarted its sequence space:
	// acceptance starts from a zero state, the only case where a lower
	// sequence is accepted.
	base := state
	if message.Epoch != state.Epoch {
		base = State{Rules: []propagation.Rule{}}
	}

	if message.Sequence <= base.LastSequence {
		return nack("stale sequence")
	}

	next := State{
		Epoch:        message.Epoch,
		LastSequence: message.Sequence,
		Rules:        []propagation.Rule{},
	}

	switch message.Kind {
	case propagation.KindPackage:
		if message.PolicyID <= 0 {
			return nack("invalid policy id")
		}

		if strings.TrimSpace(message.PolicyName) == "" {
			return nack("empty policy name")
		}

		if message.PolicyVersion <= 0 {
			return nack("invalid policy version")
		}

		if message.PolicyType != propagation.PolicyTypeWhitelist && message.PolicyType != propagation.PolicyTypeBlacklist {
			return nack("invalid policy type")
		}

		if len(message.Rules) == 0 {
			return nack("empty policy")
		}

		for _, rule := range message.Rules {
			if err := rules.Validate(rules.Rule{
				Type:     rule.Type,
				Value:    rule.Value,
				Protocol: rule.Protocol,
				Port:     rule.Port,
			}); err != nil {
				return nack(err.Error())
			}
		}

		policyID := message.PolicyID
		next.PolicyID = &policyID
		next.PolicyName = message.PolicyName
		next.PolicyVersion = message.PolicyVersion
		next.PolicyType = message.PolicyType
		next.Rules = append(next.Rules, message.Rules...)

	case propagation.KindRevoke:
		// The revoke keeps the new epoch and sequence and clears the policy
		// metadata and rules: the sequence must survive, or older packages
		// would be accepted after a restart.

	default:
		return nack("unknown message kind")
	}

	if err := runner.store.Save(next); err != nil {
		return nack("failed to persist state")
	}

	return next, propagation.Ack{Sequence: message.Sequence, Status: propagation.AckAcked}
}
