GOOS	  = windows
GOARCH	  = amd64

GO		  = go
GO_BUILD  = $(GO) build

LDFLAGS   = -w -s

NAME	  = gochan
ENTRY	  = ./cmd/$(NAME)
BINDIR    = ./bin/

.PHONY: build

build:
	$(GO_BUILD) -o $(BINDIR) -ldflags='$(LDFLAGS)' $(ENTRY)
	@echo FINISHED!

