package hubdb

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Mongo struct {
	c *mongo.Collection
}

func NewMongo(db *mongo.Database, collectionName string) *Mongo {
	return &Mongo{db.Collection(collectionName)}
}

func (db *Mongo) StoreLocation(id string, netAddr string) error {
	_, err := db.c.UpdateOne(context.Background(), bson.M{
		"_id": id,
	}, bson.M{"$set": bson.M{
		"_id":  id,
		"addr": netAddr,
	}}, options.Update().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("failed to store hub location: %w", err)
	}

	return nil
}

func (db *Mongo) LoadLocation(idPrefix string) (string, error) {
	res := db.c.FindOne(context.Background(),
		bson.M{"_id": bson.M{"$regex": "^" + regexp.QuoteMeta(idPrefix)}})

	var data struct {
		Addr string `bson:"addr"`
	}

	err := res.Decode(&data)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", fmt.Errorf("%w: %w", ErrNotFound, err)
		}

		return "", fmt.Errorf("failed to load hub location: %w", err)
	}

	return data.Addr, nil
}
