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
	"sort"
	"strings"
	"time"
)

type Config struct {
	Person string
}

var c = &Config{}

const (
	stateInit = iota + 1
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
var postSkipFunc = (func())(nil)

type Message struct {
	From string
	Date time.Time
	P    string
}

type Thread struct {
	Participants string
	Date         time.Time
	Messages     []*Message
}

type Threads []*Thread

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

	threads, err := filter(c, reader)
	if err != nil {
		return
	}

	threads.Print(os.Stdout)
}

func filter(c *Config, reader io.Reader) (threads Threads, err error) {
	z := html.NewTokenizer(reader)
	ts := Threads(make([]*Thread, 0, 8))
	for {
		var err error
		ts, err = filterOne(c, reader, z, ts)

		switch err {
		case nil:
			threads = ts
		case io.EOF:
			return threads, nil
		default:
			return nil, err
		}
	}

	return threads, nil
}

func filterOne(
	c *Config, reader io.Reader, z *html.Tokenizer, threads Threads) (
	threadsOut Threads, err error) {

	tt := z.Next()
	token := html.Token{}

	switch tt {
	case html.ErrorToken:
		return nil, z.Err()

	case html.StartTagToken:
		token = z.Token()
		level++

	case html.EndTagToken:
		token = z.Token()
		level--
	}
	// log.Printf("filterOne: %v %v:%v", level, tt, token)

	switch state {
	case stateInit:
		switch {
		case tt == html.StartTagToken &&
			token.DataAtom == atom.Div &&
			hasAttr(token, "class", "thread"):

			state = stateThread

		default:
			// skip
		}

	case stateThread:
		// len(threads) must be >0
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End thread", state)
			state = stateInit

		case tt == html.TextToken:
			participants := string(z.Text())
			if strings.Contains(participants, c.Person) {
				log.Printf("%v: Begin thread %q", state, participants)
				t := &Thread{
					Date:         time.Now(),
					Participants: participants,
				}
				threads = append(threads, t)
			} else {
				log.Printf("%v: Skipping thread %q", state, participants)
				state = stateInit
			}

		case tt == html.StartTagToken &&
			token.DataAtom == atom.Div &&
			hasAttr(token, "class", "message"):

			log.Printf("%v: Message", state)
			t := threads[len(threads)-1]
			t.Messages = append(t.Messages, &Message{})
			state = stateMessage

		case tt == html.StartTagToken &&
			token.DataAtom == atom.P:

			log.Printf("%v: Message P", state)
			state = stateMessageP

		default:
			return nil, fmt.Errorf("%v: Unexpected %#v", state, token)
		}

	case stateMessage:
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End message", state)
			state = stateThread

		case token.DataAtom == atom.Div &&
			hasAttr(token, "class", "message_header"):

			log.Printf("%v: Message header", state)
			state = stateMessageHeader

		default:
			return nil, fmt.Errorf("%v: Unexpected %#v", state, token)
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
			return nil, fmt.Errorf("%v: Unexpected %+v", state, token)
		}

	case stateUser:
		t := threads[len(threads)-1]
		m := t.Messages[len(t.Messages)-1]
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End user", state)
			state = stateMessageHeader

		default:
			m.From = string(z.Text())
			m.From = strings.Split(m.From, " ")[0]
			log.Printf("%v: User %q", state, m.From)
		}

	case stateMeta:
		t := threads[len(threads)-1]
		m := t.Messages[len(t.Messages)-1]
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End meta", state)
			state = stateMessageHeader

		default:
			d, err := time.Parse(
				"Monday, January 2, 2006 at 3:04pm MST",
				string(z.Text()))
			if err != nil {
				return nil, err
			}
			m.Date = d

			if m.Date.Before(t.Date) {
				t.Date = m.Date
			}
			log.Printf("%v: Date: %v", state, m.Date)
		}

	case stateMessageP:
		t := threads[len(threads)-1]
		m := t.Messages[len(t.Messages)-1]
		switch {
		case tt == html.EndTagToken:
			log.Printf("%v: End message P", state)
			state = stateThread

		default:
			m.P = string(z.Text())
			m.P = strings.Replace(m.P, "\r", " ", -1)
			m.P = strings.Replace(m.P, "\n", " ", -1)
			log.Printf("%v: MessageP %q", state, m.P)
		}

	default:
		return nil, fmt.Errorf("%v: totally unexpected", state)
	}

	return threads, nil
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

func (t *Thread) Print(out io.Writer) {
	if t == nil || len(t.Messages) == 0 {
		return
	}

	fmt.Fprintf(out, "%v\t%v:\n", t.Date, t.Participants)
	for i := len(t.Messages); i > 0; i-- {
		m := t.Messages[i-1]
		fmt.Fprintf(out, "%v\t%v:\t%v\n", m.Date, m.From, m.P)
	}
}

type ThreadsByDate Threads

func (ts ThreadsByDate) Len() int           { return len(ts) }
func (ts ThreadsByDate) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }
func (ts ThreadsByDate) Less(i, j int) bool { return ts[i].Date.Before(ts[j].Date) }

func (ts Threads) Print(out io.Writer) {
	// sort the threads by time
	tss := make([]*Thread, len(ts))
	copy(tss, ts)
	sort.Sort(ThreadsByDate(tss))

	for _, t := range tss {
		t.Print(out)
	}
}
