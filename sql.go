package main

import (
  "database/sql"
  _ "github.com/mattn/go-sqlite3"
  "sync"
)

func Initialize(db *sql.DB) error {
  _, err := db.Exec(`
    CREATE TABLE test_cases (
      name TEXT PRIMARY KEY UNIQUE NOT NULL,
      frontend_endpoint TEXT NOT NULL,
      new_codepath INTEGER NOT NULL,
      result_set_size TEXT NOT NULL,
      query TEXT NOT NULL,
      count INTEGER NOT NULL,
      query_trigger TEXT NOT NULL,
      repo TEXT NOT NULL
    );
  `)
  if err != nil {
    return err
  }

  _, err = db.Exec(`
    CREATE TABLE results (
      test_case TEXT NOT NULL,
      took INTEGER NOT NULL,
      result_count INTEGER NOT NULL,
      error TEXT,
      FOREIGN KEY(test_case) REFERENCES test_cases(name)
    );
  `)
  return err
}

var insertTestStmtOnce sync.Once
var insertTestStmt *sql.Stmt

func insertTest(db *sql.DB, tc *TestCase) (err error) {
  insertTestStmtOnce.Do(func() {
    insertTestStmt, err = db.Prepare(`
      INSERT INTO test_cases (
        name, frontend_endpoint, new_codepath, repo,
        result_set_size, query, count, query_trigger
      ) VALUES (
        ?, ?, ?, ?,
        ?, ?, ?, ?
      );
    `)
  })
  if err != nil {
    return err
  }

  _, err = insertTestStmt.Exec(
    tc.Name, tc.BuildOptions["endpoints"], tc.BuildOptions["codePath"], tc.BuildOptions["repo"],
    tc.BuildOptions["resultSetSize"], tc.Query(), tc.BuildOptions["count"], tc.BuildOptions["queryTrigger"],
  );
  return err
}

var insertResultStmtOnce sync.Once
var insertResultStmt *sql.Stmt

func insertResult(db *sql.DB, tc *TestCase, res *result) (err error) {
  insertResultStmtOnce.Do(func() {
    insertResultStmt, err = db.Prepare(`
      INSERT INTO results (
        test_case, took, result_count, error
      ) VALUES (
        ?, ?, ?, ?
      );
    `)
  })
  if err != nil {
    return err
  }

  var resError *string
  if res.Err != nil {
    e := res.Err.Error()
    resError = &e
  }
  _, err = insertResultStmt.Exec(
    tc.Name, res.Took, res.ResultCount, resError,
  );
  return err
}
