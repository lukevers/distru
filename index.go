package main

import (
	"bufio"
	"encoding/json"
	"github.com/temoto/robotstxt.go"
	"io"
	"log"
	"net"
	"net/http"
	"time"
)

const (
	BotName = "Distru"
)

type Index struct {
	Sites map[string]*site //A map of fully indexed webpages.
	Cache []*page          //Pages that recently turned up in the search results
	Queue chan string      `json:"-"` //The channel which controls Indexers
}

type site struct {
	Time  time.Time        //The time when the site finished indexing
	Pages map[string]*page //Nonordered map of pages on the server
	Links []string         //List of all unique links collected from all pages on the site
}

type page struct {
	Title       string         //The contents of the <title> tag
	Description string         //The description of the page
	Time        time.Time      //The time that this page was either indexed or recieved from another instance
	Link        string         //The fully qualified link to this page
	WordCount   map[string]int //Counts for every plaintext word on the webpage
}

//Index.MergeRemote makes a raw distru request for the JSON encoded index of the given site, (which must have a full URI.) It will not overwrite local sites with remote ones unless trustNew is true. Additionally, it will time out the connection, and not modify the Index, after the number of seconds given in timeout. (0 will cause it to use net.Dial() normally.) It returns nil if successful, or returns an error if the remote site could not be reached, or produced an invalid index.
func (index *Index) MergeRemote(remote string, trustNew bool, timeout int) (err error) {
	//Dial the connection here.
	var conn net.Conn
	if timeout == 0 {
		conn, err = net.Dial("tcp", remote+":9049")
		if err != nil {
			return
		}
	} else {
		conn, err = net.DialTimeout("tcp", remote+":9049", time.Duration(4)*time.Second)
	}
	defer conn.Close()

	//Initialize a new reader.
	r := bufio.NewReader(conn)

	//Create a new decoder for reading the JSON
	//directly off the wire.
	dec := json.NewDecoder(r)

	//Create a place for the decoder to decode
	//into.
	var remoteIndex Index

	//Upon initializing the connection, the
	//remote will immediately serve its JSON
	//index. We will decode into the
	//remoteIndex object.
	err = dec.Decode(&remoteIndex)
	if err != nil {
		return
	}

	isPresent := false
	for k, v := range remoteIndex.Sites {
		//If the site in the local index is not present, or if
		//the remote index is trusted, *and* newer than the
		//local one, add the remote site to the local index.
		_, isPresent = index.Sites[k]
		if !isPresent || (trustNew && index.Sites[k].Time.Before(v.Time)) {
			index.Sites[k] = v
		}
		//Repeat until we've gone through all of the
		//values in remoteIndex.
	}
	return nil
}

//Index.JSON writes a JSON-encoded (encoding/json) stream to the provided io.Writer. (This can be a Conn object.)
func (index *Index) JSON(w io.Writer) error {
	//Create an encoder for the io.Writer.
	enc := json.NewEncoder(w)
	return enc.Encode(index)
}

//MaintainIndex launches a number of goroutines which handle indexing of sites in sequence. It sets index.Queue to a channel into which target urls should be placed. When a new string is added to the returned chan, one of the next non-busy indexer will remove it from the chan and index it, and add the contents to the passed index. It will then forget about that site.
//To remove a site from the index, use delete(index.Sites, urlstring). To shut down the indexers, close() index.Queue.
func MaintainIndex(index *Index, numIndexers int) {
	//First, we're going to make the channel of pending sites.
	index.Queue = make(chan string)

	//Next, we're going to launch numIndexers amount of Indexers.
	for i := 0; i < numIndexers; i++ {
		go Indexer(index, index.Queue)
	}
}

func Indexer(index *Index, pending <-chan string) {
	for target := range pending {
		log.Println("indexer> adding \"" + target + "\"")
		newSite := newSite(target)
		if newSite == nil {
			//If we got an error for some reason,
			log.Println("indexer> failed to add \"" + target + "\"")
			//discard it and continue.
			continue
		}
		//Update the target site.
		index.Sites[target] = newSite
		log.Println("indexer> added \"" + target + "\"")
	}
}

func newSite(target string) *site {
	target = "http://" + target

	//Create an http.Client to control the webpage requests.
	client := http.Client{}
	//Use robotstxt to get the search engine permission.
	rperm, _ := getRobotsPermission(target)

	//Check if we are allowed to access /
	if !rperm.Test("/") {
		//If we aren't, return empty.
		return nil
	}

	pages := make(map[string]*page)
	newPage, tree, links := getPage(target, "/", client)
	if newPage == nil {
		//If we didn't get the root page properly,
		//return nil.
		return nil
	}
	//If we did get it, then continue normally.
	pages["/"] = newPage
	//Grab the root page first, then we're going to build on the tree.
	//We'll loop until there are no more unresolved pages. Then we'll
	//set isFinished to true, and break the loop.

	pool := make(chan string, 16) //This chan will contain new paths to index
	status := make(chan bool, 1)  //This chan will be passed true if a pager is beginning to index, and false if it has finished
	workchan := make(chan int, 0) //This chan will be used by the worker handler to signal to the main for loop that it has just recieved an update

	//Initialize some number of pagers
	var pagers int
	if len(tree) < 16 {
		pagers = len(tree)
	} else {
		pagers = 16
	}

	go func(workchan chan<- int, status <-chan bool) {
		var working int
		for {
			update, ok := <-status
			if !ok {
				return
			}
			//If update is true, then a
			//routine has started work.
			//If it is false, then the
			//opposite is true.
			if update == true {
				working += 1
			} else {
				working -= 1
			}
			workchan <- working
		}
	}(workchan, status)

	for i := 0; i < pagers; i++ {
		go pager(pool, status, target, client, rperm, pages, links)
	}

	for v := range tree {
		pool <- v
	}

	for working := range workchan {
		//If the number of working pagers has just
		//dropped to zero, and there are no queued
		//elements,
		// (Thanks http://stackoverflow.com/questions/13003749)
		//then we can safely close the pool to
		//terminate the workers, and the manager.
		if working == 0 && len(pool) == 0 && len(status) == 0 && len(workchan) == 0 {
			close(pool)
			close(status)
			close(workchan)
			break
		}
		//If the pool buffer is full, we start
		//16 more pagers.
		if len(pool) == cap(pool) {
			for i := 0; i < 16; i++ {
				go pager(pool, status, target, client, rperm, pages, links)
			}
		}
	}

	linkArray := make([]string, 0, len(links))
	for k, _ := range links {
		linkArray = append(linkArray, k)
	}

	site := &site{
		Time:  time.Now(),
		Pages: pages,
		Links: linkArray,
	}
	return site
}

func pager(pool chan string, status chan<- bool, target string, client http.Client, rperm *robotstxt.Group, pages map[string]*page, links map[string]struct{}) {
	for {
		path, ok := <-pool
		if !ok {
			return
		}
		//When we begin, we must signal that.
		status <- true
		//Block the page from other indexing.
		pages[path] = nil
		if !rperm.Test(path) {
			//If the page has been indexed already,
			//or if we're not allowed to access it,
			//ignore it.
			delete(pages, path)
			status <- false
			continue
		}
		//Then we index the page and grab the new tree.
		newPage, newTree, newLinks := getPage(target, path, client)
		if newPage == nil {
			//If we got a nil response from getPage,
			//then continue and drop this page
			delete(pages, path)
			status <- false
			continue
		}
		//If we got a good response, then put it in the map.
		pages[path] = newPage

		//Then we put the new links into the old map,
		for k, v := range newLinks {
			links[k] = v
		}
		//and put all of the unindexed parts of the tree into the pool,
		for k, _ := range newTree {
			if pages[k] != nil {
				continue
			}
			pool <- k
		}
		//and start the loop over again.
		status <- false
	}
}

func getRobotsPermission(target string) (*robotstxt.Group, error) {
	//We're going to define a routine with which to fail.
	fail := func(err error) (*robotstxt.Group, error) {
		//Since we're failing here when there is no file available,
		//craft a stand-in one to be parsed instead.
		robots, _ := robotstxt.FromBytes([]byte("User-agent: *\nAllow: /"))
		return robots.FindGroup(BotName), err
	}
	//Use robotstxt here.
	resp, err := http.Get(target + "/robots.txt")
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	robots, err := robotstxt.FromResponse(resp)
	if err != nil {
		return fail(err)
	}
	group := robots.FindGroup(BotName)
	if group == nil {
		//BUG(DuoNoxSol): This does not raise a real error.
		return fail(nil)
	}
	return group, nil
}
