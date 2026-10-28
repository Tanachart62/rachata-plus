package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Read one database snapshot so totals and detail lists agree.
func dashboardHandler(db *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		actor, ok := requireSession(c, db)
		if !ok {
			return
		}
		if actor.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin_required"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			accountFailure(c, err)
			return
		}
		defer rollback(tx)
		var users, members, suspended, total, published, draft, hidden, pending, processing, ready, failed int64
		err = tx.QueryRow(ctx, `SELECT count(*),
   count(*) FILTER (WHERE u.account_status='active' AND EXISTS (
    SELECT 1 FROM subscriptions s WHERE s.user_id=u.id AND s.status='active' AND s.starts_at<=NOW() AND s.expires_at>NOW())),
   count(*) FILTER (WHERE u.account_status='suspended') FROM users u WHERE u.account_status<>'deleted'`).Scan(&users, &members, &suspended)
		if err != nil {
			accountFailure(c, err)
			return
		}
		err = tx.QueryRow(ctx, `SELECT count(*),
   count(*) FILTER (WHERE publication_status='published'), count(*) FILTER (WHERE publication_status='draft'),
   count(*) FILTER (WHERE publication_status='hidden'), count(*) FILTER (WHERE status='pending'),
   count(*) FILTER (WHERE status='processing'), count(*) FILTER (WHERE status='ready'),
   count(*) FILTER (WHERE status='failed') FROM videos WHERE deleted_at IS NULL`).Scan(&total, &published, &draft, &hidden, &pending, &processing, &ready, &failed)
		if err != nil {
			accountFailure(c, err)
			return
		}
		jobs := make([]gin.H, 0)
		rows, err := tx.Query(ctx, `SELECT id,title,status,updated_at FROM videos WHERE deleted_at IS NULL AND status IN ('pending','processing','failed')
   ORDER BY CASE status WHEN 'processing' THEN 0 WHEN 'pending' THEN 1 ELSE 2 END,created_at,id LIMIT 10`)
		if err != nil {
			accountFailure(c, err)
			return
		}
		for rows.Next() {
			var id int64
			var title, status string
			var at time.Time
			if err = rows.Scan(&id, &title, &status, &at); err != nil {
				break
			}
			jobs = append(jobs, gin.H{"id": id, "title": title, "status": status, "updatedAt": at.UnixMilli()})
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			accountFailure(c, err)
			return
		}
		activities := make([]gin.H, 0)
		// Video timestamps describe the latest row state, not an invented audit history.
		rows, err = tx.Query(ctx, `SELECT kind,label,detail,at FROM (
   SELECT 'account'::text AS kind, e.action AS label,
    COALESCE(NULLIF(a.name,''),'ผู้ดูแลระบบ') || ' → ' || CASE WHEN u.account_status='deleted' THEN 'บัญชีที่ลบแล้ว' ELSE u.name END AS detail,
    e.created_at AS at,e.id AS tie FROM membership_events e JOIN users u ON u.id=e.user_id JOIN users a ON a.id=e.actor_id
   UNION ALL SELECT 'video',title,CASE WHEN deleted_at IS NOT NULL THEN 'deleted' ELSE status || ':' || publication_status END,updated_at,id FROM videos
   ) events ORDER BY at DESC,kind,tie DESC LIMIT 20`)
		if err != nil {
			accountFailure(c, err)
			return
		}
		for rows.Next() {
			var kind, label, detail string
			var at time.Time
			if err = rows.Scan(&kind, &label, &detail, &at); err != nil {
				break
			}
			activities = append(activities, gin.H{"kind": kind, "label": label, "detail": detail, "at": at.UnixMilli()})
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			accountFailure(c, err)
			return
		}
		if err = tx.Commit(ctx); err != nil {
			accountFailure(c, err)
			return
		}
		c.JSON(200, gin.H{"actorId": actor.ID, "asOf": time.Now().UnixMilli(),
			"accounts": gin.H{"total": users, "members": members, "suspended": suspended},
			"videos":   gin.H{"total": total, "published": published, "draft": draft, "hidden": hidden},
			"queue":    gin.H{"pending": pending, "processing": processing, "ready": ready, "failed": failed},
			"jobs":     jobs, "activities": activities})
	}
}
