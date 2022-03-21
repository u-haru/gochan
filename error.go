package gochan

import "errors"

var (
	ErrInvalidServer = errors.New("invalid Server")

	ErrInvalidBBS   = errors.New("invalid bbs")
	ErrBBSExists    = errors.New("bbs exists")
	ErrBBSNotExists = errors.New("bbs not exists")

	ErrInvalidKey   = errors.New("invalid key")
	ErrKeyExists    = errors.New("key exists")
	ErrKeyNotExists = errors.New("key not exists")

	ErrInvalidRes   = errors.New("invalid res")
	ErrResNotExists = errors.New("res not exists")
)
