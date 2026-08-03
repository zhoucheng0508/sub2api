package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/model"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// TLSFingerprintRouter stores ordered inbound User-Agent routing rules.
type TLSFingerprintRouter struct{ ent.Schema }

func (TLSFingerprintRouter) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "tls_fingerprint_routers"}}
}

func (TLSFingerprintRouter) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }

func (TLSFingerprintRouter) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(100).NotEmpty().Unique(),
		field.Text("description").Optional().Nillable(),
		field.Bool("enabled").Default(true),
		field.JSON("rules", []model.TLSFingerprintRouterRule{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
	}
}
