package account

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/socialos/backend/internal/adapters/mail"
	"github.com/socialos/backend/internal/domain/dataexport"
)

// purgeBatch bounds how many rows one statement deletes, so no transaction runs long or holds many locks.
const purgeBatch = 200

// Purge deletes everything the user owns: first the objects and rows with many entries, in batches, then the user row
// (which cascades to what is left). It is safe to run again after a failure or alongside another run: each step only
// deletes what is still there. A post being published or an export being built makes it return an error, so the queue
// retries later, instead of deleting under a running job.
func (s *DeletionService) Purge(ctx context.Context, userID uuid.UUID) error {
	now := s.d.Clock.Now()
	email, ok, err := s.d.Repo.Claim(ctx, userID, now)
	if err != nil {
		return err
	}
	if !ok {
		return nil // cancelled, not due yet, or already gone
	}
	// The account may have been given work between the request and now (a post scheduled before access ended).
	if _, err := s.d.Posts.UnscheduleAll(ctx, userID); err != nil {
		return fmt.Errorf("stop scheduled posts: %w", err)
	}
	if why, err := s.d.Repo.Busy(ctx, userID); err != nil {
		return err
	} else if why != "" {
		return fmt.Errorf("purge of user %s must wait: %s", userID, why)
	}
	counts, err := s.d.Repo.Counts(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.d.Repo.SaveCounts(ctx, userID, counts); err != nil {
		return err
	}
	// Posts before media (post_media.media_id is ON DELETE RESTRICT) and before social accounts (post_targets has no cascade).
	for _, t := range []Table{TablePosts, TableAuditLogs, TableAnalytics, TableApprovals} {
		if err := s.drain(ctx, userID, t); err != nil {
			return err
		}
	}
	if err := s.purgeMedia(ctx, userID); err != nil {
		return err
	}
	if err := s.purgeExports(ctx, userID); err != nil {
		return err
	}
	if s.d.Prefixes != nil {
		if err := s.d.Prefixes.DeletePrefix(ctx, dataexport.UserPrefix(userID)); err != nil {
			return fmt.Errorf("delete remaining objects: %w", err)
		}
	}
	if err := s.d.Repo.DeleteUser(ctx, userID, s.d.Clock.Now()); err != nil {
		return err
	}
	s.d.Log.InfoContext(ctx, "account purged", slog.String("user_id", userID.String()),
		slog.Int64("posts", counts.Posts), slog.Int64("media", counts.Media), slog.Int64("social_accounts", counts.SocialAccounts))
	s.notify(ctx, email, mail.AccountDeleted, mail.Data{})
	return nil
}

func (s *DeletionService) drain(ctx context.Context, userID uuid.UUID, t Table) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := s.d.Repo.DeleteBatch(ctx, userID, t, purgeBatch)
		if err != nil {
			return fmt.Errorf("delete %s: %w", t, err)
		}
		if n < purgeBatch {
			return nil
		}
	}
}

// purgeMedia deletes each batch's objects from storage, then its rows. An object is gone before its row, so a failure
// never leaves a file nobody can find; a missing object is not an error.
func (s *DeletionService) purgeMedia(ctx context.Context, userID uuid.UUID) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch, err := s.d.Repo.MediaBatch(ctx, userID, purgeBatch)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(batch))
		for i, m := range batch {
			if err := s.d.Store.Delete(ctx, m.Key); err != nil {
				return fmt.Errorf("delete media object %s: %w", m.ID, err)
			}
			ids[i] = m.ID
		}
		if err := s.d.Repo.DeleteMedia(ctx, userID, ids); err != nil {
			return err
		}
	}
}

func (s *DeletionService) purgeExports(ctx context.Context, userID uuid.UUID) error {
	keys, err := s.d.Repo.ExportKeys(ctx, userID)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := s.d.Store.Delete(ctx, k); err != nil {
			return fmt.Errorf("delete export object: %w", err)
		}
	}
	return nil
}
