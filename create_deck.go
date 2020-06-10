package libmtgfail

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/avast/retry-go"
	"github.com/guygrigsby/mtgfail"
	"github.com/inconshreveable/log15"
)

// BuildDeck ...
func BuildDeck(ctx context.Context, client *firestore.Client, deckList map[string]int, log log15.Logger) (*Deck, error) {
	var (
		deck = Deck{
			Cards: nil,
		}
		docRefs []*firestore.DocumentRef
	)
	cards := client.Collection("cards")

	for name := range deckList {
		docRefs = append(docRefs, cards.Doc(name))

	}
	snapshot, err := client.GetAll(ctx, docRefs)
	if err != nil {
		log.Error(
			"can't get cards",
			"err", err,
		)
	}

	ch := make(chan *CardShort)
	ech := make(chan error)
	for _, ref := range snapshot {
		ref := ref
		go func() error {
			var card *CardShort
			err := ref.DataTo(card)
			if err != nil {
				log.Error(
					"Cannot extract Data",
					"err", err,
					"ref", ref,
				)
				return err
			}
			ch <- card
			return nil
		}()

	}
	for range snapshot {
		select {
		case c := <-ch:
			deck.Cards = append(deck.Cards, c)
		case err := <-ech:
			close(ch)
			close(ech)
			log.Error(
				"can't get cards",
				"err", err,
			)
			return nil, err
		}
	}

	return &deck, nil

}

// ConvertToPairText ...
func ConvertToPairText(deck *Deck) (map[string]int, error) {
	cards := make(map[string]int)
	if len(deck.Cards) == 0 {
		return nil, fmt.Errorf("Zero length deck %+v", deck)
	}
	for _, card := range deck.Cards {
		count := cards[card.Name]
		count++
		cards[card.Name] = count
	}
	return cards, nil
}

// FetchDeck ...
func FetchDeck(u *url.URL, log log15.Logger) (io.ReadCloser, error, int) {
	var (
		content io.ReadCloser
		deckURI string
	)

	switch u.Host {
	//https://tappedout.net/mtg-decks/22-01-20-kess-storm/
	case "tappedout.net":
		deckURI = fmt.Sprintf("%s?fmt=txt", deckURI)
		log.Debug(
			"tappedout",
			"deckUri", deckURI,
		)
		var res *http.Response
		err := retry.Do(
			func() error {
				var err error
				c := http.Client{
					Timeout: 5 * time.Second,
				}
				res, err = c.Get(deckURI)
				if err != nil {
					return err
				}
				return nil
			},
			retry.Attempts(3),
		)
		if err != nil {
			log.Error(
				"cannot get tappedout deck",
				"err", err,
				"uri", deckURI,
			)
			return nil, fmt.Errorf("Cannot get tappedout deck: %w", err), http.StatusServiceUnavailable
		}
		if res.StatusCode != 200 {
			log.Error(
				"Unexpected response status",
				"status", res.Status,
			)
			return nil, fmt.Errorf("Unexpected status code from tappedout"), http.StatusBadRequest

		}
		content = res.Body

	// https://deckbox.org/sets/2649137
	case "deckbox.org":
		deckURI = fmt.Sprintf("%s/export", deckURI)
		log.Debug(
			"deckbox",
			"deckUri", deckURI,
		)
		var res *http.Response
		err := retry.Do(
			func() error {
				var err error
				res, err = http.DefaultClient.Get(deckURI)
				if err != nil {
					return err
				}
				return nil
			})
		if err != nil {
			log.Error(
				"cannot get deckbox deck",
				"err", err,
				"uri", deckURI,
			)
			return nil, err, http.StatusServiceUnavailable
		}
		if res.StatusCode != 200 {
			log.Error(
				"Unexpected response status",
				"status", res.Status,
			)
			return nil, fmt.Errorf("unexpected status %v", res.StatusCode), http.StatusBadGateway

		}

		content, err = mtgfail.Normalize(res.Body, log)
		if err != nil {
			log.Error(
				"Unexpected format for deck status",
				"err", err,
				"url", deckURI,
			)
			return nil, err, http.StatusBadGateway
		}
		break

	default:
		log.Debug(
			"Unexpected deck Host",
			"url", deckURI,
			"Host", u.Host,
		)

		return nil, fmt.Errorf("Unknown Host"), http.StatusUnprocessableEntity
	}
	return content, nil, 200
}

type Deck struct {
	Cards []*CardShort
}
type CardShort struct {
	Name   string   `firestore:"name"`
	Cost   string   `firestore:"cost"`
	Cmc    float64  `firestore:"cmc"`
	Image  string   `firestore:"image"`
	Rarity string   `firestore:"rarity"`
	Set    string   `firestore:"set"`
	Colors []string `firestore:"colors"`
	Text   string   `firestore:"oracle_text"`
}

type Bulk map[string]*Entry

type Entry struct {
	Object          string        `json:"object"`
	ID              string        `json:"id"`
	OracleID        string        `json:"oracle_id"`
	MultiverseIds   []interface{} `json:"multiverse_ids"`
	Name            string        `json:"name"`
	Lang            string        `json:"lang"`
	ReleasedAt      string        `json:"released_at"`
	URI             string        `json:"uri"`
	ScryfallURI     string        `json:"scryfall_uri"`
	Layout          string        `json:"layout"`
	HighresImage    bool          `json:"highres_image"`
	ImageUris       ImageUris     `json:"image_uris"`
	ManaCost        string        `json:"mana_cost"`
	Cmc             float64       `json:"cmc"`
	TypeLine        string        `json:"type_line"`
	OracleText      string        `json:"oracle_text"`
	Colors          []string      `json:"colors"`
	ColorIdentity   []string      `json:"color_identity"`
	CardFaces       []CardFace    `json:"card_faces,omitempty"`
	Legalities      Legalities    `json:"legalities"`
	Games           []string      `json:"games"`
	Reserved        bool          `json:"reserved"`
	Foil            bool          `json:"foil"`
	Nonfoil         bool          `json:"nonfoil"`
	Oversized       bool          `json:"oversized"`
	Promo           bool          `json:"promo"`
	Reprint         bool          `json:"reprint"`
	Variation       bool          `json:"variation"`
	Set             string        `json:"set"`
	SetName         string        `json:"set_name"`
	SetType         string        `json:"set_type"`
	SetURI          string        `json:"set_uri"`
	SetSearchURI    string        `json:"set_search_uri"`
	ScryfallSetURI  string        `json:"scryfall_set_uri"`
	RulingsURI      string        `json:"rulings_uri"`
	PrintsSearchURI string        `json:"prints_search_uri"`
	CollectorNumber string        `json:"collector_number"`
	Digital         bool          `json:"digital"`
	Rarity          string        `json:"rarity"`
	CardBackID      string        `json:"card_back_id"`
	Artist          string        `json:"artist"`
	ArtistIds       []string      `json:"artist_ids"`
	IllustrationID  string        `json:"illustration_id"`
	BorderColor     string        `json:"border_color"`
	Frame           string        `json:"frame"`
	FullArt         bool          `json:"full_art"`
	Textless        bool          `json:"textless"`
	Booster         bool          `json:"booster"`
	StorySpotlight  bool          `json:"story_spotlight"`
	EdhrecRank      int           `json:"edhrec_rank"`
	RelatedUris     RelatedUris   `json:"related_uris"`
}
type CardFace struct {
	Object         string    `json:"object"`
	Name           string    `json:"name"`
	ManaCost       string    `json:"mana_cost"`
	TypeLine       string    `json:"type_line"`
	OracleText     string    `json:"oracle_text"`
	Colors         []string  `json:"colors"`
	Power          string    `json:"power"`
	Toughness      string    `json:"toughness"`
	Artist         string    `json:"artist"`
	ArtistID       string    `json:"artist_id"`
	IllustrationID string    `json:"illustration_id"`
	ImageUris      ImageUris `json:"image_uris"`
}
type ImageUris struct {
	Small      string `json:"small"`
	Normal     string `json:"normal"`
	Large      string `json:"large"`
	Png        string `json:"png"`
	ArtCrop    string `json:"art_crop"`
	BorderCrop string `json:"border_crop"`
}
type Legalities struct {
	Standard  string `json:"standard"`
	Future    string `json:"future"`
	Historic  string `json:"historic"`
	Pioneer   string `json:"pioneer"`
	Modern    string `json:"modern"`
	Legacy    string `json:"legacy"`
	Pauper    string `json:"pauper"`
	Vintage   string `json:"vintage"`
	Penny     string `json:"penny"`
	Commander string `json:"commander"`
	Brawl     string `json:"brawl"`
	Duel      string `json:"duel"`
	Oldschool string `json:"oldschool"`
}
type RelatedUris struct {
	TcgplayerDecks string `json:"tcgplayer_decks"`
	Edhrec         string `json:"edhrec"`
	Mtgtop8        string `json:"mtgtop8"`
}
type AutoComplete struct {
	Object      string   `json:"object"`
	TotalValues int      `json:"total_values"`
	Data        []string `json:"data"`
}
