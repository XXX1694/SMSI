package identity

// DisplayName is the provider's name as users know it.
func (p Provider) DisplayName() string {
	switch p {
	case Google:
		return "Google"
	case GitHub:
		return "GitHub"
	default:
		return string(p)
	}
}

// Message is the sentence shown to the user when a flow is refused, in plain words and with the next step.
func (r Reason) Message(p Provider) string {
	switch r {
	case ReasonEmailUnverified:
		if p == GitHub {
			return "GitHub did not give us a verified primary email address. Verify your primary email on GitHub, then try again."
		}
		return p.DisplayName() + " did not give us a verified email address. Verify it with " + p.DisplayName() + ", then try again."
	case ReasonAccountExists:
		return "An account with this email already exists. Sign in with your password, then connect " + p.DisplayName() + " in Settings."
	case ReasonAccountUnavailable:
		return "This account is not available."
	default:
		return "Sign-in failed. Please try again."
	}
}
