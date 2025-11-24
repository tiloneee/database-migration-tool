package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Order holds the schema definition for the Order entity.
type Order struct {
	ent.Schema
}

// Fields of the Order.
func (Order) Fields() []ent.Field {
	return []ent.Field{
		field.Int("user_id"),
		field.Float("total_amount").
			Positive(),
		field.String("status").
			Default("pending"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Order.
func (Order) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("orders").
			Field("user_id").
			Unique().
			Required(),
		edge.To("items", OrderItem.Type),
	}
}

// Indexes of the Order.
func (Order) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
