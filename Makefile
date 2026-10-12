.PHONY: test vet serve-demo eval-demo eval-scifact fetch-scifact benchmark build

test:
	go test -race ./...
vet:
	go vet ./...
serve-demo:
	go run ./cmd/evidence-investigator -mode=serve -data=data/demo
eval-demo:
	go run ./cmd/evidence-investigator -mode=eval -data=data/demo -k=5
fetch-scifact:
	go run ./cmd/evidence-investigator -mode=fetch -data=data/scifact
eval-scifact:
	go run ./cmd/evidence-investigator -mode=eval -data=data/scifact -k=5
benchmark:
	go run ./cmd/evidence-investigator -mode=benchmark -data=data/scifact -k=5 -limit=10
build:
	go build -trimpath -o evidence-investigator ./cmd/evidence-investigator
