package wiki

import (
	"context"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/liamphmurphy/vesper/server/internal/store"
)

// Content keys are scoped to the uploader. The same lock serializes a
// re-upload with deletion of an older, unreferenced object of that hash.
func (s *Service) StoreKnowledgeAsset(ctx context.Context, ws, user uuid.UUID, hash, filename, typ string, size int64, reader io.Reader) (store.KnowledgeAsset, error) {
	objects, bucket, err := s.vaultObjects()
	if err != nil {
		return store.KnowledgeAsset{}, err
	}
	key := fmt.Sprintf("knowledge-assets/%s/%s/%s", ws, user, hash)
	var asset store.KnowledgeAsset
	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := q.LockKnowledgeBlob(ctx, key); err != nil {
			return err
		}
		if err := objects.Put(ctx, bucket, key, reader, size, typ); err != nil {
			return err
		}
		var err error
		asset, err = q.PutKnowledgeAsset(ctx, store.PutKnowledgeAssetParams{WorkspaceID: ws, OwnerUserID: user, ContentHash: hash, BlobKey: key, Filename: filename, ContentType: typ, SizeBytes: size})
		if err != nil {
			return err
		}
		return q.ClearKnowledgeAssetDeletion(ctx, key)
	})
	return asset, err
}

// Deletion uses persisted tombstones. A database failure or S3 outage cannot
// make the sweeper forget an orphan; a concurrent re-upload cancels deletion.
func (s *Service) SweepKnowledgeAssets(ctx context.Context) error {
	objects, bucket, err := s.vaultObjects()
	if err != nil {
		return err
	}
	q := store.New(s.Pool)
	if err = q.StageOrphanKnowledgeAssets(ctx); err != nil {
		return err
	}
	keys, err := q.ListKnowledgeAssetDeletions(ctx)
	if err != nil {
		return err
	}
	for _, item := range keys {
		err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
			q := store.New(tx)
			if err := q.LockKnowledgeBlob(ctx, item.BlobKey); err != nil {
				return err
			}
			used, err := q.KnowledgeBlobInUse(ctx, item.BlobKey)
			if err != nil {
				return err
			}
			if !used {
				if err = objects.Delete(ctx, bucket, item.BlobKey); err != nil {
					return err
				}
			}
			return q.ClearKnowledgeAssetDeletion(ctx, item.BlobKey)
		})
		if err != nil {
			return err
		}
	}
	archives, err := q.FinishedKnowledgeArchives(ctx)
	if err != nil {
		return err
	}
	for _, a := range archives {
		if err = objects.Delete(ctx, bucket, a.BlobKey); err != nil {
			return err
		}
		if err = q.MarkKnowledgeArchiveDeleted(ctx, a.ID); err != nil {
			return err
		}
	}
	return nil
}
