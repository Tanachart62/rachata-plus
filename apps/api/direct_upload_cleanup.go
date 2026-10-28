package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"time"
)

func (s *videoService) cleanDirectUploads(ctx context.Context, conn rowExecutor) error {
	// Expired sessions cannot complete. Lock each row against a completion that
	// started before expiry, and keep every command on the worker owner connection.
	rows, err := conn.Query(ctx, `SELECT id FROM direct_uploads WHERE expires_at<NOW() ORDER BY expires_at LIMIT 10`)
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			break
		}
		keys = append(keys, key)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, key := range keys {
		beginner, ok := conn.(transactionBeginner)
		if !ok {
			return nil
		}
		cleanup, stop := context.WithTimeout(ctx, 30*time.Second)
		err = func() error {
			tx, err := beginner.Begin(cleanup)
			if err != nil {
				return err
			}
			defer rollback(tx)
			var uploadID string
			var videoID *int64
			err = tx.QueryRow(cleanup, `SELECT upload_id,video_id FROM direct_uploads WHERE id=$1 AND expires_at<NOW() FOR UPDATE SKIP LOCKED`, key).Scan(&uploadID, &videoID)
			if err != nil {
				return nil
			}
			if videoID == nil && storageKeyPattern.MatchString(key) {
				_, err = s.store.client.AbortMultipartUpload(cleanup, &s3.AbortMultipartUploadInput{Bucket: aws.String(s.store.bucket), Key: aws.String(s.store.key(key + "/source")), UploadId: aws.String(uploadID)})
				if err != nil {
					var missing *types.NoSuchUpload
					if !errors.As(err, &missing) {
						return err
					}
				}
				if err = s.store.deletePrefix(cleanup, key); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(cleanup, `DELETE FROM direct_uploads WHERE id=$1`, key); err != nil {
				return err
			}
			return tx.Commit(cleanup)
		}()
		stop()
		if err != nil {
			return err
		}
	}
	_, err = conn.Exec(ctx, `DELETE FROM request_limits WHERE key_hash IN(SELECT key_hash FROM request_limits WHERE resets_at<NOW() LIMIT 1000) AND resets_at<NOW()`)
	if err != nil {
		return err
	}
	return s.cleanDeletedAvatars(ctx, conn)
}

func (s *videoService) cleanDeletedAvatars(ctx context.Context, conn rowExecutor) error {
	rows, err := conn.Query(ctx, `SELECT id FROM users WHERE account_status='deleted' AND avatar_s3_key IS NOT NULL LIMIT 10`)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = s.store.deletePrefix(ctx, fmt.Sprintf("avatars/%d", id)); err != nil {
			return err
		}
		if _, err = conn.Exec(ctx, `UPDATE users SET avatar_s3_key=NULL WHERE id=$1 AND account_status='deleted'`, id); err != nil {
			return err
		}
	}
	return nil
}
