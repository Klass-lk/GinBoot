package model

import (
	"time"
)

type Post struct {
	ID        string    `ginboot:"id" bson:"_id,omitempty" json:"id"`
	Title     string    `bson:"title" json:"title"`
	Content   string    `bson:"content" json:"content"`
	Author    string    `bson:"author" json:"author"`
	Tags      []string  `bson:"tags" json:"tags"`
	CreatedAt time.Time `bson:"created_at" json:"createdAt"`
	UpdatedAt time.Time `bson:"updated_at" json:"updatedAt"`
}
