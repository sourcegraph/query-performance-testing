package main

import (
	"context"
  "database/sql"
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
  BuildOptions map[string]string
	Name           string
	Endpoints      Endpoints
	UseNewCodepath bool
	Repo           string
	ResultSetSize  string
	Count          int
	QueryTrigger   QueryTrigger
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

type QueryTrigger struct {
  Count int 
  Interval time.Duration
}

func (q *QueryTrigger) C() <-chan time.Time {
  c := make(chan time.Time, 1000)
  c <- time.Now()
  go func() {
    timer := time.NewTicker(q.Interval)
    defer timer.Stop()
    defer close(c)
    for i := 0; i < q.Count-1; i++ {
      t := <-timer.C
      c <- t
    }
  }()
  return c
}

func (q *QueryTrigger) ProfileTime() time.Duration {
		return time.Duration(q.Count)*q.Interval + 5*time.Second
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
    return []*TestCase{{BuildOptions:make(map[string]string)}}
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
      tc.BuildOptions[remaining[0]] = name
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
		t.QueryTrigger = QueryTrigger{
      Count: count,
      Interval: interval,
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

func runTest(tc *TestCase, dir string, db *sql.DB) {
	log.Printf("Running case %s", tc.Name)
	log.Printf("Query %s", tc.Query())
	log.Printf("Expected time %s", tc.QueryTrigger.ProfileTime())

  if err := insertTest(db, tc); err != nil {
    log.Fatalf("Insert test case: %s", err)
  }

	dir = dir + "/" + tc.Name
	if err := os.Mkdir(dir, 0755); err != nil {
		log.Fatalf("failed to create test dir: %s", err)
	}

	var wg sync.WaitGroup

	if tc.Endpoints.FrontendDebugEndpoint != "" {
		wg.Add(1)
		go func() {
      collectTrace(tc.Endpoints.FrontendDebugEndpoint, dir+"/frontend.trace", tc.QueryTrigger.ProfileTime())
			wg.Done()
		}()

		wg.Add(1)
		go func() {
      collectProfile(tc.Endpoints.FrontendDebugEndpoint, dir+"/frontend.prof", tc.QueryTrigger.ProfileTime())
			wg.Done()
		}()
	}

	if tc.Endpoints.SearcherDebugEndpoint != "" {
		wg.Add(1)
		go func() {
			collectTrace(tc.Endpoints.SearcherDebugEndpoint, dir+"/searcher.trace", tc.QueryTrigger.ProfileTime())
			wg.Done()
		}()

		wg.Add(1)
		go func() {
			collectProfile(tc.Endpoints.SearcherDebugEndpoint, dir+"/searcher.prof", tc.QueryTrigger.ProfileTime())
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
	mc := make(chan *result, 1000)
	i := 0
  for range tc.QueryTrigger.C() {
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

	wg.Add(1)
	go func() {
		for result := range mc {
      err := insertResult(db, tc, result)
      if err != nil {
        log.Fatalf("Insert result: %s", err)
      }
		}
		wg.Done()
	}()

	wg.Wait()
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

type result struct {
	Took        int64
	ResultCount int
	Err         error
}

func collectResults(tc *TestCase, client *client, dir string, i int, mc chan *result) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	results, metrics, err := client.search(ctx, tc.Query())
	if err != nil {
		log.Printf("failed running search: %s", err)
		mc <- &result{Err: err}
		return
	}
	log.Printf("Got %d results in %d milliseconds", results.Search.Results.ResultCount, metrics.took)

	mc <- &result{
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

  db, err := sql.Open("sqlite3", dir+"/results.sqlite")
  if err != nil {
    log.Fatalf("Open db: %s", err)
  }
  if err := Initialize(db); err != nil {
    log.Fatalf("Initialize db: %s", err)
  }

	cases := matrix.Iter()
	for i, tc := range cases {
		log.Printf("Running test %d of %d", i+1, len(cases))
    print(tc.BuildOptions)
		clearCache()
		runTest(tc, dir, db)
		time.Sleep(1)
	}
}
