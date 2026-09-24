package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
)

func main() {
	execute := flag.Bool("execute", false, "refund outstanding holds and remove the legacy media-video schema")
	confirmed := flag.Bool(
		"confirm-all-instances-upgraded",
		false,
		"confirm that every instance using the legacy media-video schema has exited",
	)
	flag.Parse()

	if *execute && !*confirmed {
		log.Fatal("--execute requires --confirm-all-instances-upgraded")
	}

	cfg, err := config.LoadForBootstrap()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	client, db, err := repository.InitEnt(cfg)
	if err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	preview, err := repository.PreviewLegacyMediaVideoCleanup(ctx, db)
	if err != nil {
		log.Fatalf("preview cleanup: %v", err)
	}
	fmt.Printf(
		"legacy_schema=%t refundable_tasks=%d refundable_users=%d refundable_amount=%.8f pending_balance_cache_users=%d\n",
		preview.LegacySchemaFound,
		preview.RefundedTasks,
		preview.RefundedUsers,
		preview.RefundedAmount,
		preview.PendingBalanceCacheUsers,
	)
	if !*execute {
		fmt.Println("dry-run only; no balances or schema objects were changed")
		return
	}

	redisClient := repository.InitRedis(cfg)
	defer func() { _ = redisClient.Close() }()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis must be reachable before cleanup: %v", err)
	}

	result, err := repository.FinalizeLegacyMediaVideoCleanup(ctx, db)
	if err != nil {
		log.Fatalf("finalize cleanup: %v", err)
	}
	invalidated, err := repository.InvalidateLegacyMediaVideoBalanceCaches(
		ctx,
		db,
		repository.NewBillingCache(redisClient),
	)
	if err != nil {
		log.Fatalf("cleanup committed but balance cache invalidation remains pending; rerun the same command: %v", err)
	}

	fmt.Printf(
		"cleanup_complete legacy_schema=%t refunded_tasks=%d refunded_users=%d refunded_amount=%.8f invalidated_balance_cache_users=%d\n",
		result.LegacySchemaFound,
		result.RefundedTasks,
		result.RefundedUsers,
		result.RefundedAmount,
		invalidated,
	)
}
