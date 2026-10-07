package accounts

import (
	"context"

	"github.com/google/uuid"
	"github.com/socialos/backend/internal/adapters/provider"
	"github.com/socialos/backend/internal/application/port"
	"github.com/socialos/backend/internal/domain/errs"
	"github.com/socialos/backend/internal/domain/socialaccount"
)

// Vault encrypts and decrypts OAuth credentials. Plaintext tokens exist only in memory.
type Vault struct {
	repo Repo
	enc  port.Encryptor
}

// NewVault creates a vault.
func NewVault(repo Repo, enc port.Encryptor) *Vault { return &Vault{repo: repo, enc: enc} }

// Save encrypts and stores tokens for an account. An empty refresh token in
// tok keeps the previously stored one (providers often omit it on refresh).
func (v *Vault) Save(ctx context.Context, accountID uuid.UUID, tok provider.Token) error {
	access, err := v.enc.Encrypt(tok.AccessToken)
	if err != nil {
		return err
	}
	refresh, err := v.enc.Encrypt(tok.RefreshToken)
	if err != nil {
		return err
	}
	c := EncryptedCredentials{AccessTokenEnc: access, RefreshTokenEnc: refresh, ExpiresAt: tok.ExpiresAt,
		RefreshExpiresAt: tok.RefreshExpiresAt, KeyVersion: v.enc.KeyVersion()}
	if tok.RefreshToken == "" {
		if prev, err := v.repo.GetCredentials(ctx, accountID); err == nil {
			c.RefreshTokenEnc, c.RefreshExpiresAt = prev.RefreshTokenEnc, prev.RefreshExpiresAt
		}
	}
	return v.repo.SaveCredentials(ctx, accountID, c)
}

// Load decrypts credentials. Accounts without stored tokens return empty credentials.
func (v *Vault) Load(ctx context.Context, accountID uuid.UUID) (socialaccount.Credentials, error) {
	c, err := v.repo.GetCredentials(ctx, accountID)
	if errs.Is(err, errs.NotFound) {
		return socialaccount.Credentials{}, nil
	}
	if err != nil {
		return socialaccount.Credentials{}, err
	}
	access, err := v.enc.Decrypt(c.AccessTokenEnc)
	if err != nil {
		return socialaccount.Credentials{}, errs.Wrap(errs.Internal, "cannot decrypt credentials", err)
	}
	refresh, err := v.enc.Decrypt(c.RefreshTokenEnc)
	if err != nil {
		return socialaccount.Credentials{}, errs.Wrap(errs.Internal, "cannot decrypt credentials", err)
	}
	return socialaccount.Credentials{AccessToken: access, RefreshToken: refresh, ExpiresAt: c.ExpiresAt, RefreshExpiresAt: c.RefreshExpiresAt}, nil
}
