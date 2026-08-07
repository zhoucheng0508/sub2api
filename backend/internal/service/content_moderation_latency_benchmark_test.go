package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func benchmarkContentModerationConversation(turnCount int) ContentModerationInput {
	turns := make([]ContentModerationTurn, 0, turnCount*2)
	for index := 0; index < turnCount; index++ {
		purpose := "supporting_context"
		if index == turnCount-1 {
			purpose = "audit_target"
		}
		turns = append(turns,
			ContentModerationTurn{
				Role:    "user",
				Text:    fmt.Sprintf("normal user request %d %s", index, strings.Repeat("context ", 24)),
				Purpose: purpose,
				Source:  "request_body",
			},
			ContentModerationTurn{
				Role:    "assistant",
				Text:    fmt.Sprintf("normal assistant response %d %s", index, strings.Repeat("response ", 24)),
				Purpose: "supporting_context",
				Source:  "request_body",
			},
		)
	}
	target := turns[len(turns)-2].Text
	return ContentModerationInput{
		Text:            target,
		CurrentText:     target,
		AuditTargetKind: "latest_user",
		AuditTargetText: target,
		HasExplicitUser: true,
		Turns:           turns,
	}
}

func BenchmarkContentModerationIncrementalPreparation(b *testing.B) {
	for _, turnCount := range []int{1, 10, 40, 100, 512} {
		content := benchmarkContentModerationConversation(turnCount)
		cfg := defaultContentModerationConfig()
		cfg.AuditProvider = ContentModerationProviderAIChat
		cfg.AIChat.IncrementalAuditEnabled = true
		cfg.normalize()
		svc := &ContentModerationService{}
		input := ContentModerationCheckInput{}

		b.Run(fmt.Sprintf("turns_%d/fast_only", turnCount), func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				plan, err := svc.prepareIncrementalAudit(context.Background(), input, cfg, content)
				if err != nil || plan == nil || plan.reviewInputBuilt {
					b.Fatalf("prepare fast audit: plan=%v err=%v", plan, err)
				}
			}
		})

		b.Run(fmt.Sprintf("turns_%d/full_review", turnCount), func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				plan, err := svc.prepareIncrementalAudit(context.Background(), input, cfg, content)
				if err != nil {
					b.Fatal(err)
				}
				plan.ensureReviewInput(cfg, false, nil)
				if !plan.reviewInputBuilt || strings.TrimSpace(plan.fullInput) == "" {
					b.Fatal("full review input was not built")
				}
			}
		})
	}
}
