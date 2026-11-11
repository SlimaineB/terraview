.PHONY: build run static clean

BINARY := terraview
SRC := main.go

build:
	go build -o $(BINARY) $(SRC)

run: build
	go run $(SRC)

static:
	go build -o $(BINARY) $(SRC)
	./$(BINARY) -static -in plan.json -out terraview-report.html

clean:
	rm -f $(BINARY) terraview-report.html

.DEFAULT_GOAL := build
