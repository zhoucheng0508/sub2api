# Legacy media-video cleanup

The automatic migration only creates cleanup bookkeeping. It deliberately keeps
the legacy task tables, triggers, functions, and `groups.video_price_per_request`
column so old and new application instances can overlap during a rolling update.

After every instance running the database-backed media-video implementation has
exited, preview the cleanup from the backend directory:

```shell
go run ./cmd/finalize-legacy-media-video
```

Preview uses the application's normal database bootstrap, which may apply pending
migrations or initialize required secrets. It does not refund legacy media-video
holds or remove the legacy schema. Review the task, user, and refund totals. Then
execute the finalizer:

```shell
go run ./cmd/finalize-legacy-media-video \
  --execute \
  --confirm-all-instances-upgraded
```

The command locks the retired task table, refunds only holds that have a durable
hold event without a capture or release event, enqueues API-key authentication
cache invalidations, removes the retired schema, and invalidates each affected
user's balance cache. Refund and cache progress is recorded in
`legacy_media_video_cleanup_refunds`, so a Redis or process failure can be
recovered by rerunning the same command.

Do not execute the finalizer while an old application instance is running. A
five-second database lock timeout makes the command fail instead of waiting
indefinitely when the retired table is still busy.
