.PHONY: stop start check_mattermost build
	
logs:
	@echo "Following logs..."
	@docker-compose logs --follow
	@echo "Done"

setup-mattermost:
	@cp ./files/mattermost/copilotDefaults.json ./volumes/mattermost/config
	@cp ./files/mattermost/samlCert.crt ./volumes/mattermost/config
	@cp ./license.mattermost ./volumes/mattermost/config/license.mattermost-enterprise

restore-keycloak:
	@./scripts/keycloak.sh restore

build-apps:
	@echo "Building app containers..."
	@docker-compose build weather-app flightaware-app
	@echo "App containers built successfully"

run: 
	@echo "Starting..."
	@make restore-keycloak
	@make run-core
	@make setup-mattermost
	@go run ./main.go wait-for-start
	@make run-rtcd
	#@docker exec -it mattermost mmctl user create --email user@example.com --username sysadmin --password Sys@dmin-sample1 --system-admin --email-verified --local
	@go run ./main.go setup --verbose --ldap --config ./config.json
	@make echo-logins

run-ai:
	@echo "Setting up copilot"
	@docker exec -it mattermost mmctl plugin add /ai-plugins/mattermost-ai.tar.gz --local --force
	@docker exec -it mattermost mmctl plugin add /ai-plugins/mattermost-channel-translations.tar.gz --force --local
	@docker exec -it mattermost mmctl plugin enable mattermost-ai --local
	@docker exec -it mattermost mmctl plugin enable mattermost-channel-translations --local
	@docker exec -it mattermost mmctl config patch /mattermost/config/copilotDefaults.json --local
	@docker exec -it mattermost mmctl plugin disable mattermost-ai --local
	@docker exec -it mattermost mmctl plugin disable mattermost-channel-translations --local
	@docker exec -it mattermost mmctl plugin enable mattermost-ai --local
	@docker exec -it mattermost mmctl plugin enable mattermost-channel-translations --local
	@echo "Copilot Should be up and running. Go crazy."

run-core:
	@echo "Starting the core services... hang in there."
	@docker-compose up -d postgres openldap prometheus grafana elasticsearch mattermost keycloak

run-rtcd:
	@echo "Starting RTCD..."
	@docker-compose up -d mattermost-rtcd
	@docker exec -it mattermost mmctl plugin disable com.mattermost.calls --local
	@docker exec -it mattermost mmctl plugin enable com.mattermost.calls --local

start:
	@echo "Starting the existing deployment..."
	@docker-compose start
	
stop:
	@echo "Stopping..."
	@docker-compose stop
	@echo "Done"

restart:
	@docker-compose restart
	@make check-mattermost

reset:
	@echo "Resetting..."
	@make delete-data
	@make start

delete-dockerfiles:
	@echo "Deleting data..."
	@docker-compose rm
	@rm -rf ./volumes
	@rm -rf ./files/postgres/replica/replica_*
	@echo "Done"

delete-data: stop delete-dockerfiles

purge:
	@echo "Purging containers and volumes..."
	@docker-compose down --volumes --remove-orphans
	@make delete-data
	@echo "Done"

nuke: 
	@echo "Nuking Docker..."
	@docker-compose down --volumes --remove-orphans
	@make delete-data
	@echo "Removing app images..."
	@docker rmi demo-kit-weather-app demo-kit-flightaware-app demo-kit-missionops-app 2>/dev/null || true

echo-logins:
	@go run ./main.go echo-logins

GO_PACKAGES=$(shell go list ./...)
GO ?= $(shell command -v go 2> /dev/null)

BUILD_COMMAND ?= go build -ldflags '$(LDFLAGS)' -o ./bin/mmsetup 

# We need to export GOBIN to allow it to be set
# for processes spawned from the Makefile
export GOBIN ?= $(PWD)/bin

build: test
	mkdir -p bin
	$(BUILD_COMMAND)

# run:
# 	go run ./main.go

package: test
	mkdir -p build bin 

	@echo Build Linux amd64
	env GOOS=linux GOARCH=amd64 $(BUILD_COMMAND)
	tar cf - -C bin mmhealth | gzip -9 > build/linux_amd64.tar.gz


	@echo Build OSX amd64
	env GOOS=darwin GOARCH=amd64 $(BUILD_COMMAND)
	tar cf - -C bin mmhealth | gzip -9 > build/darwin_amd64.tar.gz

	@echo Build OSX arm64
	env GOOS=darwin GOARCH=arm64 $(BUILD_COMMAND)
	tar cf - -C bin mmhealth | gzip -9 > build/darwin_arm64.tar.gz

	@echo Build Windows amd64
	env GOOS=windows GOARCH=amd64 go build -ldflags '$(LDFLAGS)' -o ./bin/mmhealth.exe 
	zip -9 build/windows_amd64.zip ./bin/mmhealth.exe

	rm ./bin/mmhealth ./bin/mmhealth.exe

check-style:  install-go-tools verify-gomod
	@echo Running golangci-lint
	$(GO) vet ./...
	$(GOBIN)/golangci-lint run ./...

test: check-style
	@echo Running tests
	$(GO) test -race -cover -v $(GO_PACKAGES)

verify-gomod:
	$(GO) mod download
	$(GO) mod verify

## Install go tools
install-go-tools:
	@echo Installing go tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6
