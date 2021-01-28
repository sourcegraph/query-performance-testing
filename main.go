package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// A map of repo => result set size => pattern
var matchPatterns = map[string]map[string]string{
	`github\.com/sourcegraph/sourcegraph-typescript$`: {
		"small":  "ProgressProvider = (:[1])", // 2 results
		"medium": "const :[1] =",              // 213 results
		"large":  "(:[1])",                    // 407 results
	},
	`torvalds/linux$`: {
		"small":  "balloon_page_enqueue_one(:[1])",   // 3 results
		"medium": "#include <crypto/internal/:[1]> ", // 196 results
		"large":  "notify(:[1])",                     // 470 results
	},
	`^(github.com/)?chromium/chromium$`: {
		"small":  "izip(:[1])",             // 14 results
		"medium": "DCHECK_LT(index, :[1])", // 151 results
		"large":  "base::size(:[1])",       // 703 results
	},
}

type TestCase struct {
	Name           string
	Endpoints      Endpoints
	UseNewCodepath bool
	Repo           string
	ResultSetSize  string
	Count          int
	QueryTrigger   func() <-chan time.Time
	ProfileTime    time.Duration
}

func (t *TestCase) MatchPattern() string {
	repo, ok := matchPatterns[t.Repo]
	if !ok {
		panic("invalid repo")
	}

	matchPattern, ok := repo[t.ResultSetSize]
	if !ok {
		panic("invalid result set size")
	}

	return matchPattern
}

func (t *TestCase) Query() string {
	var rule string
	if t.UseNewCodepath {
		rule = ` rule:'where "zoekt" == "zoekt"'`
	}
	return fmt.Sprintf(`timeout:10m count:%d patternType:structural repo:%s %s %s`, t.Count, t.Repo, rule, t.MatchPattern())
}

type Endpoints struct {
	FrontendEndpoint      string
	FrontendDebugEndpoint string
	SearcherDebugEndpoint string
	Token                 string
}

type OptMatrix map[string]map[string]Opt

func (o OptMatrix) Iter() []*TestCase {
	return iterRecursive(o, o.GroupNames())
}

func (o OptMatrix) GroupNames() []string {
	groupNames := make([]string, 0, len(o))
	for groupName := range o {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)
	return groupNames
}

func iterRecursive(o OptMatrix, remaining []string) []*TestCase {
	if len(remaining) == 0 {
		return []*TestCase{{}}
	}
	cases := make([]*TestCase, 0)
	for name, opt := range o[remaining[0]] {
		sub := iterRecursive(o, remaining[1:])
		for _, tc := range sub {
			opt(tc)
			if tc.Name == "" {
				tc.Name = name
			} else {
				tc.Name = fmt.Sprintf("%s_%s", name, tc.Name)
			}
		}
		cases = append(cases, sub...)
	}
	return cases
}

type Opt func(*TestCase)

func EndpointOpt(frontend, frontendDebug, searcherDebug, tokenEnv string) Opt {
	token := os.Getenv(tokenEnv)
	if token == "" {
		panic(fmt.Sprintf("Env %s does not contain token", tokenEnv))
	}
	return func(t *TestCase) {
		t.Endpoints = Endpoints{
			FrontendEndpoint:      frontend,
			FrontendDebugEndpoint: frontendDebug,
			SearcherDebugEndpoint: searcherDebug,
			Token:                 token,
		}
	}
}

func CodePathOpt(useNewCodepath bool) Opt {
	return func(t *TestCase) {
		t.UseNewCodepath = useNewCodepath
	}
}

func RepoOpt(repo string) Opt {
	return func(t *TestCase) {
		t.Repo = repo
	}
}

func CountOpt(count int) Opt {
	return func(t *TestCase) {
		t.Count = count
	}
}

func ResultSetSizeOpt(size string) Opt {
	return func(t *TestCase) {
		t.ResultSetSize = size
	}
}

func QueryTriggerOpt(interval time.Duration, count int) Opt {
	return func(t *TestCase) {
		t.ProfileTime = time.Duration(count)*interval + 5*time.Second
		t.QueryTrigger = func() <-chan time.Time {
			log.Printf("Expecting %d result sets", count)
			c := make(chan time.Time, 1000)
			c <- time.Now()
			go func() {
				timer := time.NewTicker(interval)
				defer timer.Stop()
				defer close(c)
				for i := 0; i < count-1; i++ {
					t := <-timer.C
					c <- t
				}
			}()
			return c
		}
	}
}

func getTestDir() string {
	dir := fmt.Sprintf("run_%s", time.Now().Format(time.RFC3339))
	if err := os.Mkdir(dir, 0755); err != nil {
		panic(err)
	}
	return dir
}

func runTest(tc *TestCase, dir string) {
	log.Printf("Running case %s", tc.Name)
	log.Printf("Query %s", tc.Query())
	log.Printf("Expected time %s", tc.ProfileTime)

	dir = dir + "/" + tc.Name
	if err := os.Mkdir(dir, 0755); err != nil {
		log.Fatalf("failed to create test dir: %s", err)
	}

	var wg sync.WaitGroup

	if tc.Endpoints.FrontendDebugEndpoint != "" {
		wg.Add(1)
		go func() {
			collectTrace(tc.Endpoints.FrontendDebugEndpoint, dir+"/frontend.trace", tc.ProfileTime)
			wg.Done()
		}()

		wg.Add(1)
		go func() {
			collectProfile(tc.Endpoints.FrontendDebugEndpoint, dir+"/frontend.prof", tc.ProfileTime)
			wg.Done()
		}()
	}

	if tc.Endpoints.SearcherDebugEndpoint != "" {
		wg.Add(1)
		go func() {
			collectTrace(tc.Endpoints.SearcherDebugEndpoint, dir+"/searcher.trace", tc.ProfileTime)
			wg.Done()
		}()

		wg.Add(1)
		go func() {
			collectProfile(tc.Endpoints.SearcherDebugEndpoint, dir+"/searcher.prof", tc.ProfileTime)
			wg.Done()
		}()
	}

	// Wait a second after starting profiling
	time.Sleep(time.Second)

	// Queries
	var resultsWg sync.WaitGroup
	client, err := newClient(tc.Endpoints.FrontendEndpoint, tc.Endpoints.Token)
	if err != nil {
		log.Fatalf("failed to get client: %s", err)
	}
	mc := make(chan *stats, 1000)
	i := 0
	for range tc.QueryTrigger() {
		resultsWg.Add(1)
		go func() {
			i := i
			collectResults(tc, client, dir, i, mc)
			resultsWg.Done()
		}()
		i++
	}

	wg.Add(1)
	go func() {
		resultsWg.Wait()
		close(mc)
		wg.Done()
	}()

	agg := make([]*stats, 0, 100)
	wg.Add(1)
	go func() {
		for stats := range mc {
			agg = append(agg, stats)
		}
		wg.Done()
	}()

	wg.Wait()
	writeMetrics(agg, dir)
}

func writeMetrics(agg []*stats, dir string) {
	f, err := os.Create(dir + "/metrics.json")
	if err != nil {
		log.Fatalf("failed to create metrics file: %s", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	if err := enc.Encode(agg); err != nil {
		log.Fatalf("failed to write metrics: %s", err)
	}
}

func collectTrace(endpoint, path string, dur time.Duration) {
	resp, err := http.Get(fmt.Sprintf(
		"http://%s/debug/pprof/trace?seconds=%d",
		endpoint,
		int(dur.Seconds()),
	))
	if err != nil {
		log.Printf("failed to collect trace: %s", err)
		return
	}
	defer resp.Body.Close()

	f, err := os.Create(path)
	if err != nil {
		log.Printf("failed to open trace.out: %s", err)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Printf("failed to write to trace.out: %s", err)
	}
}

func collectProfile(endpoint, path string, dur time.Duration) {
	resp, err := http.Get(fmt.Sprintf(
		"http://%s/debug/pprof/profile?seconds=%d",
		endpoint,
		int(dur.Seconds()),
	))
	if err != nil {
		log.Printf("failed to collect profile: %s", err)
		return
	}
	defer resp.Body.Close()

	f, err := os.Create(path)
	if err != nil {
		log.Printf("failed to open profile.out: %s", err)
		return
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		log.Printf("failed to write to profile.out: %s", err)
	}

}

type stats struct {
	Took        int64
	ResultCount int
	Err         error
}

func collectResults(tc *TestCase, client *client, dir string, i int, mc chan *stats) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	results, metrics, err := client.search(ctx, tc.Query())
	if err != nil {
		log.Printf("failed running search: %s", err)
		mc <- &stats{Err: err}
		return
	}
	log.Printf("Got %d results in %d milliseconds", results.Search.Results.ResultCount, metrics.took)

	mc <- &stats{
		Took:        metrics.took,
		ResultCount: results.Search.Results.ResultCount,
	}
}

func clearCache() {
	os.RemoveAll("/tmp/searcher-archives")
}

func main() {
	matrix := OptMatrix{
		"endpoints": {
			"local": EndpointOpt("http://127.0.0.1:3080", "127.0.0.1:6063", "127.0.0.1:6069", "LOCAL_TOKEN"),
			"cloud": EndpointOpt("https://sourcegraph.com", "", "", "CLOUD_TOKEN"),
		},
		"codePath": {
			"new": CodePathOpt(true),
			"old": CodePathOpt(false),
		},
		"repo": {
			"sgtest":   RepoOpt(`github\.com/sourcegraph/sourcegraph-typescript$`),
			"linux":    RepoOpt(`torvalds/linux$`),
			"chromium": RepoOpt(`^(github.com/)?chromium/chromium$`),
		},
		"resultSetSize": {
			"sm": ResultSetSizeOpt("small"),
			"md": ResultSetSizeOpt("medium"),
			"lg": ResultSetSizeOpt("large"),
		},
		"count": {
			"10":    CountOpt(10),
			"10000": CountOpt(10000),
		},
		"queryTrigger": {
			"2x5s":   QueryTriggerOpt(5*time.Second, 2),
			"20x05s": QueryTriggerOpt(500*time.Millisecond, 20),
		},
	}

	dir := getTestDir()
	log.Printf("Saving results to %s", dir)
	log.Printf("Field order: %v", matrix.GroupNames())

	cases := matrix.Iter()
	for i, tc := range cases {
		log.Printf("Running test %d of %d", i+1, len(cases))
		clearCache()
		runTest(tc, dir)
		time.Sleep(1)
	}
}
