package main

import (
	"context"
	"io/ioutil"

	"cloud.google.com/go/storage"
	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/file"
	"google.golang.org/appengine/log"
)

const (
	maxDatastoreRecordsToDelete = 500
)

func getStringFromFile(path string) string {
	file, err := ioutil.ReadFile(path)

	if err != nil {
		return ""
	}

	return string(file)
}

func storeCSV(ctx context.Context, fileName string, data []byte) (err error) {
	err = storeFile(ctx, fileName, "text/csv", data)

	return
}

func storeFile(ctx context.Context, fileName, contentType string, data []byte) (err error) {
	var bucketName string

	if appengine.IsDevAppServer() {
		return
	}

	bucketName, err = file.DefaultBucketName(ctx)
	if err != nil {
		log.Errorf(ctx, "[STORAGE] Failed to get default GCS bucket name: %v", err)
		return
	}

	client, err := storage.NewClient(ctx)

	defer client.Close()

	if err != nil {
		log.Errorf(ctx, "[STORAGE] Failed to create client: %v", err)
		return
	}

	bucket := client.Bucket(bucketName)

	w := bucket.Object(fileName).NewWriter(ctx)
	w.ContentType = contentType

	_, err = w.Write(data)
	if err != nil {
		log.Errorf(ctx, "[STORAGE] Failed to write file: %v", err)
	}

	err = w.Close()
	if err != nil {
		log.Errorf(ctx, "[STORAGE] Failed to close writer: %v", err)
	}

	return
}

func removeFromDatastore(ctx context.Context, keys []*datastore.Key) (err error) {
	keyCount := len(keys)
	remainingKeys := keyCount

	keysToRemoveIndex := 0
	keysToRemoveCount := 0

	for remainingKeys > 0 {
		keysToRemoveCount = remainingKeys
		if remainingKeys > maxDatastoreRecordsToDelete {
			keysToRemoveCount = maxDatastoreRecordsToDelete
			remainingKeys -= maxDatastoreRecordsToDelete
		} else {
			remainingKeys = 0
		}

		keysToRemove := keys[keysToRemoveIndex : keysToRemoveIndex+keysToRemoveCount]
		keysToRemoveIndex += keysToRemoveCount

		err = datastore.DeleteMulti(ctx, keysToRemove)

		if err != nil {
			return
		}
	}

	return
}
