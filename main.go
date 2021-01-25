package main

import (
  "context"
  "fmt"
  "sort"
  "log"
  "io"
  "os"
  "time"
  "net/http"
  "sync"
)


var matchPatterns = map[string]map[string]string{
  `github\.com/sgtest/sourcegraph-typescript$`: {
    "small": "",  // TODO
    "medium": "", 
    "large": "", 
  },
  `github\.com/github\.com/torvalds/linux$`: {
    "small": "", 
    "medium": "", 
    "large": "", 
  },
  // `github\.com/sgtest/sourcegraph-typescript$`: { // TODO
  //   "small": "", 
  //   "medium": "", 
  //   "large": "", 
  // },
}

type TestCase struct {
  Name string
  Endpoints Endpoints
  UseNewCodepath bool 
  Repo string
  ResultSetSize string
  QueryTrigger func() <-chan time.Time
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
  return fmt.Sprintf(`patternType:structural repo:%s %s %s`, t.Repo, rule, t.MatchPattern())
}

type Endpoints struct {
  FrontendEndpoint string
  FrontendDebugEndpoint string
  SearcherDebugEndpoint string
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
      tc.Name = fmt.Sprintf("%s%s", name, tc.Name)
    }
    cases = append(cases, sub...)
  }
  return cases
}

type Opt func(*TestCase) 

func EndpointOpt(frontend, frontendDebug, searcherDebug string) Opt {
  return func(t *TestCase) {
    t.Endpoints = Endpoints {
      FrontendEndpoint: frontend,
      FrontendDebugEndpoint: frontendDebug,
      SearcherDebugEndpoint: searcherDebug,
    }
  }
}

func CodePathOpt(useNewCodepath bool) Opt {
  return func(t *TestCase) {
    t.UseNewCodepath = useNewCodepath
  }
}

func RepoOpt( repo string ) Opt {
  return func(t *TestCase) {
    t.Repo = repo
  }
}


func ResultSetSizeOpt(size string) Opt {
  return func(t *TestCase) {
    t.ResultSetSize = size
  }
}

func QueryTriggerOpt(interval time.Duration, count int) Opt {
  return func(t *TestCase) {
    t.QueryTrigger = func() <-chan time.Time {
      c := make(chan time.Time)
      c <- time.Now()
      go func() {
        timer := time.NewTimer(interval)
        defer timer.Stop()
        defer close(c)
        for i := 0; i < count - 1; i++ {
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

  var wg sync.WaitGroup

  // Trace
  wg.Add(1)
  go func() {
    collectTrace(tc, dir, 10)
    wg.Done()
  }()

  // Profile
  wg.Add(1)
  go func() {
    collectProfile(tc, dir, 10)
    wg.Done()
  }()

  // Wait a second after starting profiling
  time.Sleep(time.Second)

  // Queries
  var resultsWg sync.WaitGroup
  client, err := newClient()
  if err != nil {
    log.Fatalf("failed to get client: %s", err)
  }
  mc := make(chan *stats, 100)
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
}

func collectTrace(tc *TestCase, dir string, seconds int) {
  resp, err := http.Get(fmt.Sprintf(
    "http://%s/debug/pprof/trace?seconds=%d", 
    tc.Endpoints.FrontendDebugEndpoint,
    seconds,
  ))
  if err != nil {
    log.Printf("failed to collect trace: %s", err)
    return
  }
  defer resp.Body.Close()

  f, err := os.Create(dir + "/trace.out")
  if err != nil {
    log.Printf("failed to open trace.out: %s", err)
    return
  }

  if _, err := io.Copy(f, resp.Body); err != nil {
    log.Printf("failed to write to trace.out: %s", err)
  }
}

func collectProfile(tc *TestCase, dir string, seconds int) {
  resp, err := http.Get(fmt.Sprintf(
    "http://%s/debug/pprof/profile?seconds=%d", 
    tc.Endpoints.FrontendDebugEndpoint,
    seconds,
  ))
  if err != nil {
    log.Printf("failed to collect profile: %s", err)
    return
  }
  defer resp.Body.Close()

  f, err := os.Create(dir + "/profile.out")
  if err != nil {
    log.Printf("failed to open profile.out: %s", err)
    return
  }

  if _, err := io.Copy(f, resp.Body); err != nil {
    log.Printf("failed to write to profile.out: %s", err)
  }

}

type stats struct {
  Took int64
  ResultCount int
}

func collectResults(tc *TestCase, client *client, dir string, i int, mc chan *stats) {
  ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
  defer cancel()

  results, metrics, err := client.search(ctx, tc.Query()) 
  if err != nil {
    log.Printf("failed running search: %s", err)
    return
  }
  log.Printf("Got %d results in %d milliseconds", results.Search.Results.ResultCount, metrics.took)

  mc <- &stats{
    Took: metrics.took,
    ResultCount: results.Search.Results.ResultCount,
  }
}

func main() {
  matrix := OptMatrix{
    "endpoints": {
      "A": EndpointOpt("localhost:TODO", "localhost:6063", "localhost:6069"),
    },
    "codePath": {
      "A": CodePathOpt(true),
      "B": CodePathOpt(false),
    },
    "repo": {
      "A": RepoOpt(`github\.com/sgtest/sourcegraph-typescript$`),
      "B": RepoOpt(`github\.com/torvalds/linux$`),
    },
    "resultSetSize": {
      "A": ResultSetSizeOpt("small"),
      "B": ResultSetSizeOpt("medium"),
      "C": ResultSetSizeOpt("large"),
    },
    "queryTrigger": {
      "A": QueryTriggerOpt(10 * time.Second, 2),
      "B": QueryTriggerOpt(time.Second, 100),
      "C": QueryTriggerOpt(100 * time.Millisecond, 1000),
      "D": QueryTriggerOpt(10 * time.Millisecond, 10000),
    },
  }

  dir := getTestDir()
  log.Printf("Saving results to %s", dir)

  cases := matrix.Iter()
  log.Printf("Running %d cases", len(cases))
  for _, tc := range cases {
    runTest(tc, dir)
    time.Sleep(1)
  } 
}

