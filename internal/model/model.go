package model

import "time"

type Word struct {
    word       string
    frequency  string
    pureEn     bool
    sample     string
    updateTime time.Time
    lastTime   time.Time
}

type Cache struct {
    word          string
    pronunciation string
    paraphrase    []string
    sentences     [][]string
}
