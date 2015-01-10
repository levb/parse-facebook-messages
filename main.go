package main

import (
	"bufio"
	"flag"
	"fmt"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	Person string
}

var c = &Config{}

func init() {
	flag.StringVar(&c.Person, "person", "", "Friend's name")
}

func main() {
	var err error
	defer func() {
		if err != nil {
			fmt.Fprintf(os.Stderr, err.Error())
			os.Exit(1)
		}
	}()

	flag.Parse()

	file := os.Stdin
	path := flag.Arg(0)
	if path != "" {
		file, err = os.Open(path)
		if err != nil {
			return
		}
		defer file.Close()
	}
	reader := bufio.NewReader(file)

	err = filter(c, reader)
	if err != nil {
		return
	}
}

func filter(c *Config, reader io.Reader) error {
	z := html.NewTokenizer(reader)

	for {
		err := filterOne(c, reader, z)
		switch err {
		case nil:
			// do nothing
		case io.EOF:
			return nil
		default:
			return err
		}
	}

	return nil
}

func hasAttr(t html.Token, key, val string) bool {
	if key == "" || val == "" {
		return true
	}
	for _, a := range t.Attr {
		if a.Key == key && a.Val == val {
			return true
		}
	}
	return false
}

func skipTo(a atom.Atom, class string, newState int) {
	state = stateSkipToElement
	skipAtom = a
	skipAttrKey = "class"
	skipAttrValue = class
	postSkipState = newState
}

const (
	stateInit = iota + 1
	stateSkipToElement
	stateThread
	stateMessage
	stateMessageHeader
	stateUser
	stateMeta
	stateMessageP
)

var state = stateInit
var level = 0
var skipAtom = atom.Atom(0)
var skipAttrKey = ""
var skipAttrValue = ""
var postSkipState = 0
var threadParticipants = ""
var messageFrom = ""
var messageDate = time.Time{}
var messageP = ""

func filterOne(c *Config, reader io.Reader, z *html.Tokenizer) error {
	tt := z.Next()
	token := html.Token{}

	switch tt {
	case html.ErrorToken:
		return z.Err()

	case html.StartTagToken:
		token = z.Token()
		level++

	case html.EndTagToken:
		token = z.Token()
		level--
	}
	// log.Printf("filterOne: %v %v:%v", level, tt, token)

	switch state {
	case stateSkipToElement:
		switch {
		case tt != html.StartTagToken:
			log.Printf("%v: Skipping %v", state, tt)
		case skipAtom != token.DataAtom:
			log.Printf("%v: Skipping %v", state, token.DataAtom)
		case !hasAttr(token, skipAttrKey, skipAttrValue):
			log.Printf("%v: Skipping %v, didn't match %q=%q",
				state, token.DataAtom,
				skipAttrKey, skipAttrValue)
		default:
			state = postSkipState
			skipAtom = 0
			skipAttrKey = ""
			skipAttrValue = ""
			postSkipState = 0
		}

	case stateInit:
		skipTo(atom.Div, "thread", stateThread)

	case stateThread:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End thread %q", state, threadParticipants)
			skipTo(atom.Div, "thread", stateThread)

		case tt == html.TextToken:
			participants := string(z.Text())
			if strings.Contains(participants, c.Person) {
				log.Printf("%v: Begin thread %q", state, participants)
				threadParticipants = participants
				skipTo(atom.Div, "message", stateMessage)
			} else {
				log.Printf("%v: Skipping thread %q", state, participants)
				skipTo(atom.Div, "thread", stateThread)
			}

		case tt == html.StartTagToken &&
			token.DataAtom == atom.Div &&
			hasAttr(token, "class", "message"):

			log.Printf("%v: Message", state)
			state = stateMessage

		case tt == html.StartTagToken &&
			token.DataAtom == atom.P:

			log.Printf("%v: Message P", state)
			messageP = ""
			state = stateMessageP

		default:
			return fmt.Errorf("%v: Unexpected %#v", state, token)
		}

	case stateMessage:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End message", state)
			state = stateThread

		case token.DataAtom == atom.Div &&
			hasAttr(token, "class", "message_header"):

			messageFrom = ""
			messageDate = time.Time{}
			log.Printf("%v: Message header", state)
			state = stateMessageHeader

		default:
			return fmt.Errorf("%v: Unexpected %#v", state, token)
		}

	case stateMessageHeader:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End message header", state)
			state = stateMessage

		case token.DataAtom == atom.Span &&
			hasAttr(token, "class", "user"):

			log.Printf("%v: Begin user", state)
			state = stateUser

		case token.DataAtom == atom.Span &&
			hasAttr(token, "class", "meta"):

			log.Printf("%v: Begin meta", state)
			state = stateMeta

		default:
			return fmt.Errorf("%v: Unexpected %+v", state, token)
		}

	case stateUser:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End user", state)
			state = stateMessageHeader

		default:
			messageFrom = string(z.Text())
			messageFrom = strings.Split(messageFrom, " ")[0]
			log.Printf("%v: User %q", state, messageFrom)
		}

	case stateMeta:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End meta", state)
			state = stateMessageHeader

		default:
			d, err := time.Parse(
				"Monday, January 2, 2006 at 3:04pm MST",
				string(z.Text()))
			if err != nil {
				return err
			}
			messageDate = d
			log.Printf("%v: Date: %v", state, messageDate)
		}

	case stateMessageP:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End message P", state)
			fmt.Printf("%v\t%v:\t%v\n", messageDate, messageFrom, messageP)
			state = stateThread

		default:
			messageP = string(z.Text())
			messageP = strings.Replace(messageP, "\r", " ", -1)
			messageP = strings.Replace(messageP, "\n", " ", -1)
			log.Printf("%v: MessageP %q", state, messageP)
		}

	default:
		return fmt.Errorf("%v: totally unexpected", state)
	}

	return nil
}
