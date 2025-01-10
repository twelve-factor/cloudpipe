package cmd

import "sync"

type PipeCallback *func(*Pipe) error

type Resource struct {
	ID             string
	Needs          []*Blueprint
	Offers         []*Blueprint
	Pipes          map[string]*Pipe
	Mutex          sync.RWMutex
	DefaultData    any
	UpdateCallback PipeCallback
}
