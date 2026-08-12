package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/tlsfingerprintrouter"
	"github.com/Wei-Shaw/sub2api/internal/model"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type tlsFingerprintRouterRepository struct{ client *ent.Client }

// CUSTOM(VOTE-AI-OPENAI-TLS): persistence for inbound User-Agent routing rules.
func NewTLSFingerprintRouterRepository(client *ent.Client) service.TLSFingerprintRouterRepository {
	return &tlsFingerprintRouterRepository{client: client}
}

func (r *tlsFingerprintRouterRepository) List(ctx context.Context) ([]*model.TLSFingerprintRouter, error) {
	rows, err := r.client.TLSFingerprintRouter.Query().Order(ent.Asc(tlsfingerprintrouter.FieldName)).All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*model.TLSFingerprintRouter, len(rows))
	for i, row := range rows {
		result[i] = tlsFingerprintRouterToModel(row)
	}
	return result, nil
}

func (r *tlsFingerprintRouterRepository) GetByID(ctx context.Context, id int64) (*model.TLSFingerprintRouter, error) {
	row, err := r.client.TLSFingerprintRouter.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return tlsFingerprintRouterToModel(row), nil
}

func (r *tlsFingerprintRouterRepository) Create(ctx context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	builder := r.client.TLSFingerprintRouter.Create().SetName(router.Name).SetEnabled(router.Enabled).SetRules(router.Rules)
	if router.Description != nil {
		builder.SetDescription(*router.Description)
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return tlsFingerprintRouterToModel(row), nil
}

func (r *tlsFingerprintRouterRepository) Update(ctx context.Context, router *model.TLSFingerprintRouter) (*model.TLSFingerprintRouter, error) {
	builder := r.client.TLSFingerprintRouter.UpdateOneID(router.ID).SetName(router.Name).SetEnabled(router.Enabled).SetRules(router.Rules)
	if router.Description != nil {
		builder.SetDescription(*router.Description)
	} else {
		builder.ClearDescription()
	}
	row, err := builder.Save(ctx)
	if err != nil {
		return nil, err
	}
	return tlsFingerprintRouterToModel(row), nil
}

func (r *tlsFingerprintRouterRepository) Delete(ctx context.Context, id int64) error {
	return r.client.TLSFingerprintRouter.DeleteOneID(id).Exec(ctx)
}

func tlsFingerprintRouterToModel(row *ent.TLSFingerprintRouter) *model.TLSFingerprintRouter {
	rules := row.Rules
	if rules == nil {
		rules = []model.TLSFingerprintRouterRule{}
	}
	return &model.TLSFingerprintRouter{
		ID: row.ID, Name: row.Name, Description: row.Description, Enabled: row.Enabled,
		Rules: rules, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}
