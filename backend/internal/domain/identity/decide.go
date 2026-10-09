package identity

import "github.com/google/uuid"

// Action is what the sign-in flow must do next.
type Action string

const (
	// SignIn signs in the user who already owns the identity.
	SignIn Action = "sign_in"
	// LinkAndSignIn attaches the identity to the existing local account, then signs in.
	LinkAndSignIn Action = "link_and_sign_in"
	// SignUp starts a pending sign-up for a new user (completed after accepting the Terms).
	SignUp Action = "sign_up"
	// Refuse ends the flow with Decision.Reason.
	Refuse Action = "refuse"
)

// Reason says why a flow was refused. The values double as the `?error=` codes shown to the user.
type Reason string

const (
	ReasonNone               Reason = ""
	ReasonAccountUnavailable Reason = "account_unavailable"
	ReasonAccountExists      Reason = "account_exists"
	ReasonEmailUnverified    Reason = "email_unverified"
)

// Owner is the user an identity or an email already belongs to, as far as linking is concerned.
type Owner struct {
	UserID uuid.UUID
	// Active is false for disabled and deleted accounts.
	Active bool
	// EmailVerified is whether the local account's address was verified. Only used for the email match.
	EmailVerified bool
}

// Decision is the outcome of Decide.
type Decision struct {
	Action Action
	// UserID is the user to sign in (SignIn, LinkAndSignIn).
	UserID uuid.UUID
	Reason Reason
	// VerifyEmail tells the caller to store the account's email as verified (LinkAndSignIn, SignUp).
	VerifyEmail bool
}

// Decide applies the linking rules for a sign-in with no session. It is pure: the caller looks up
// byIdentity (the owner of claims.Provider + claims.Subject, nil if none) and byEmail (the local account with
// claims.Email, nil if none), and acts on the result.
//
//  1. A known (provider, subject) signs its owner in, whatever the email says now; an inactive owner is refused.
//  2. A local account with the same email is linked only when the provider is authoritative for that address AND the
//     local address is already verified. Anything else is refused: linking an unverified local account would let
//     someone who registered with a victim's email first ("pre-hijacking") take over the victim's later Google sign-in,
//     and an address the provider does not vouch for proves nothing about the person.
//  3. No match starts a sign-up, but only with a verified email.
func Decide(claims Claims, byIdentity, byEmail *Owner) Decision {
	if byIdentity != nil {
		if !byIdentity.Active {
			return refuse(ReasonAccountUnavailable)
		}
		return Decision{Action: SignIn, UserID: byIdentity.UserID}
	}
	if claims.Email == "" || !claims.EmailVerified {
		// Without a verified address we can neither match a local account safely nor create one.
		if byEmail != nil {
			return refuse(ReasonAccountExists)
		}
		return refuse(ReasonEmailUnverified)
	}
	if byEmail != nil {
		if !byEmail.Active {
			return refuse(ReasonAccountUnavailable)
		}
		if claims.AuthoritativeEmail() && byEmail.EmailVerified {
			return Decision{Action: LinkAndSignIn, UserID: byEmail.UserID, VerifyEmail: true}
		}
		return refuse(ReasonAccountExists)
	}
	return Decision{Action: SignUp, VerifyEmail: claims.AuthoritativeEmail()}
}

func refuse(r Reason) Decision { return Decision{Action: Refuse, Reason: r} }
