package db

import (
	"errors"
	"fmt"

	bolt "go.etcd.io/bbolt"
)

var ErrNotFound = errors.New("not found")

type BoltDB struct {
	db *bolt.DB
}

func NewBoltDB(dbPath string) (*BoltDB, error) {
	fmt.Printf("Opening db at %s, if it doesn't exist it will be created\n", dbPath)
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, err
	}
	err = setupNodeDataBucket(db)
	if err != nil {
		return nil, err
	}
	err = setupOperatorsBucket(db)
	if err != nil {
		return nil, err
	}
	err = setupStateBucket(db)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	return &BoltDB{db}, nil
}

func (db *BoltDB) Close() error {
	return db.db.Close()
}
