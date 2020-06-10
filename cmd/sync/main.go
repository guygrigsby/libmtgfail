package main

import (
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"

	firebase "firebase.google.com/go"
	"github.com/guygrigsby/mtgfail"
	"github.com/inconshreveable/log15"
	"github.com/prometheus/common/log"
	"google.golang.org/api/option"
)

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
	opt := option.WithCredentialsFile("snackend-firebase-key.json")
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Error(
			"cannot connect to firestore",
			"err", err,
		)
		return
	}
	err = upload(context.Background(), app, cards, log)
}
func upload(ctx context.Context, app *firebase.App, bulk map[string]*mtgfail.Entry, log log15.Logger) error {
	client, err := app.Firestore(ctx)
	if err != nil {
		log.Error(
			"cannot create client",
			"err", err,
		)
		return err
	}
	cards := client.Collection("cards")
	for _, card := range bulk {
		df, wr, err := cards.Add(ctx, card)
		if err != nil {
			log.Error(
				"cannot create document",
				"err", err,
				"res", wr,
				"docref", df,
			)
			return err
		}
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
