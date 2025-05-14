.PHONY: stop start check_mattermost
	
logs:
	@echo "Following logs..."
	@docker-compose logs --follow
	@echo "Done"

setup-mattermost:
	@cp ./files/mattermost/copilotDefaults.json ./volumes/mattermost/config
	@cp ./files/mattermost/samlCert.crt ./volumes/mattermost/config
	@cp ./license.mattermost ./volumes/mattermost/config/license.mattermost-enterprise


check-mattermost:
	@make wait-for-mattermost || (echo "Mattermost server not ready yet, will try again later"; exit 0)

wait-for-mattermost:
	@echo "Waiting for Mattermost API to become available..."
	@cd mattermost && go run ./cmd/main.go -wait-for-start

restore-keycloak:
	@./scripts/keycloak.sh restore

build-apps:
	@echo "Building app containers..."
	@docker-compose build weather-app flightaware-app missionops-app
	@echo "App containers built successfully"

run: 
	@echo "Starting..."
	@make restore-keycloak
	@make run-core
	@make setup-mattermost
	@make wait-for-mattermost
	@make run-ai
	@make run-rtcd
	@make run-integrations
	@cd mattermost && go run ./cmd/main.go -setup
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

run-integrations:
	@echo "Starting the integrations..."
	@docker-compose up -d --build weather-app flightaware-app missionops-app

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
	@cd mattermost && go run ./cmd/main.go -echo-logins
	@./scripts/general.sh logins