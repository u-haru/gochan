GOOS	  = windows
GOARCH	  = amd64

GO		  = go
GO_BUILD  = $(GO) build

LDFLAGS   = -w -s

NAME	  = gochan
ENTRY	  = ./cmd/$(NAME)


.PHONY: build

build:
	$(GO_BUILD) -ldflags='$(LDFLAGS)' $(ENTRY)
	@echo FINISHED!

