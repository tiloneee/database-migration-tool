package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OrderItem holds the schema definition for the OrderItem entity.
type OrderItem struct {
	ent.Schema
}

// Fields of the OrderItem.
func (OrderItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int("order_id"),
		field.Int("product_id"),
		field.Int("quantity").
			Positive(),
		field.Float("unit_price").
			Positive(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the OrderItem.
func (OrderItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("items").
			Field("order_id").
			Unique().
			Required(),
		edge.From("product", Product.Type).
			Ref("order_items").
			Field("product_id").
			Unique().
			Required(),
	}
}

// Indexes of the OrderItem.
func (OrderItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
		index.Fields("product_id"),
	}
}
