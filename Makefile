PKGS = $(shell go list ./... | grep -v /vendor/ | grep -v /bindata)

cover:
	@mkdir -p ./coverage
	@for pkg in $(PKGS) ; do \
		go test \
			-coverpkg=$$(go list -f '{{ join .Deps "\n" }}' $$pkg | grep '^$(PACKAGE)/' | grep -v '^$(PACKAGE)/vendor/' | tr '\n' ',')$$pkg \
			-coverprofile="./coverage/`echo $$pkg | tr "/" "-"`.cover" $$pkg ;\
	done
	@gocovmerge ./coverage/*.cover > cover.out
	@go tool cover -html=cover.out

race:
	@bash -c 'for i in {1..100}; do \
		go test -race; \
	done'

cover:
	go test -coverprofile=cover.out -coverpkg=./... ./...
	@go tool cover -html=cover.out