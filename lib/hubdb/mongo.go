package hubdb

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	c *mongo.Collection
}

func NewMongo(db *mongo.Database, collectionName string) *Mongo {
	c := db.Collection(collectionName)

	_, err := c.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "createdAt", Value: -1}},
		Options: options.Index().SetExpireAfterSeconds(int32(LocationExpiration.Seconds())),
	})
	if err != nil {
		panic(err)
	}

	return &Mongo{c}
}

func (db *Mongo) StoreLocation(id string, netAddr string) error {
	_, err := db.c.UpdateOne(context.Background(), bson.M{
		"_id": id,
	}, bson.M{"$set": bson.M{
		"_id":       id,
		"addr":      netAddr,
		"createdAt": time.Now(),
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("failed to store hub location: %w", err)
	}

	return nil
}

func (db *Mongo) LoadLocation(idPrefix string) (string, string, error) {
	res := db.c.FindOne(context.Background(),
		bson.M{"_id": bson.M{"$regex": "^" + regexp.QuoteMeta(idPrefix)}},
		options.FindOne().SetSort(bson.M{"createdAt": -1}))

	var data struct {
		ID   string `bson:"_id"`
		Addr string `bson:"addr"`
	}

	err := res.Decode(&data)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", "", fmt.Errorf("%w via id prefix %w", ErrNotFound, err)
		}

		return "", "", fmt.Errorf("failed to load hub location: %w", err)
	}

	return data.Addr, data.ID, nil
}

func (db *Mongo) DeleteLocation(id string) error {
	_, err := db.c.DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("failed to delete hub location: %w", err)
	}

	return nil
}
