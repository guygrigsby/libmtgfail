package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"

	"cloud.google.com/go/firestore"
	"github.com/guygrigsby/mtgfail"
	"github.com/inconshreveable/log15"
	"github.com/prometheus/common/log"
	"google.golang.org/api/option"
)

func main() {
	BulkSync()
}

func BulkSync() {

	log := log15.New()
	res, err := http.DefaultClient.Get("https://archive.scryfall.com/json/scryfall-default-cards.json")
	if err != nil {
		log.Error(
			"get cards failed",
			"err", err,
		)
		return
	}
	defer res.Body.Close()

	cards, err := parse(res.Body)
	if err != nil {
		log.Error(
			"parse cards failed",
			"err", err,
		)
		return
	}
	ctx := context.Background()
	c := os.Getenv("FIREBASE_CONFIG")
	client, err := firestore.NewClient(ctx, "snackend", option.WithCredentialsJSON([]byte(c)))
	if err != nil {
		log.Error(
			"cannot connect to firestore",
			"err", err,
		)
		return
	}
	err = upload(context.Background(), 100, client, cards, log)
}
func upload(ctx context.Context, cc int, client *firestore.Client, bulk map[string]*mtgfail.Entry, log log15.Logger) error {

	var wg sync.WaitGroup
	ch := make(chan *mtgfail.Entry, len(bulk))
	done := make(chan struct{})
	go func() {
		for _, card := range bulk {
			ch <- card
		}
		close(ch)
		wg.Wait()
		done <- struct{}{}
	}()
	cards := client.Collection("cards")
	for i := 0; i < cc; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for card := range ch {
				name := card.Name
				if strings.Contains(name, "//") {
					name = strings.ReplaceAll(name, "//", "")
				}
				doc := cards.Doc(name)
				wr, err := doc.Set(ctx, card)
				if err != nil {
					log.Error(
						"cannot create document skipping",
						"err", err,
						"res", wr,
					)
				}

			}
		}()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
	}
	return nil
}
func parse(r io.Reader) (map[string]*mtgfail.Entry, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		log.Error(
			"Can't read file",
			"err", err,
		)
		return nil, err
	}

	var cards []*mtgfail.Entry
	err = json.Unmarshal(b, &cards)
	if err != nil {
		log.Error(
			"Can't unmarshal data",
			"err", err,
		)
		return nil, err
	}
	var bulk = make(map[string]*mtgfail.Entry)
	for i, card := range cards {
		if card == nil {
			log.Warn(
				"nil entry skipping",
				"index", i,
			)
			continue
		}
		//TODO it's gross, but scryfall adds the time of download as a param at the end and tts no likey
		card.ImageUris.Small = strings.Split(card.ImageUris.Small, "?")[0]
		card.ImageUris.Normal = strings.Split(card.ImageUris.Normal, "?")[0]
		card.ImageUris.Large = strings.Split(card.ImageUris.Large, "?")[0]
		card.ImageUris.Png = strings.Split(card.ImageUris.Png, "?")[0]
		bulk[card.Name] = card

	}
	return bulk, nil
}
