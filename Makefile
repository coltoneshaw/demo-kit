.PHONY: stop start check_mattermost
	
logs:
	@echo "Following logs..."
	@docker-compose logs --follow
	@echo "Done"

setup-mattermost:
	@cp ./files/mattermost/copilotDefaults.json ./volumes/mattermost/config
	@cp ./files/mattermost/samlCert.crt ./volumes/mattermost/config
	@cp ./license.mattermost ./volumes/mattermost/config/license.mattermost-enterprise
	@./scripts/mattermost.sh setup

check-mattermost:
	@./scripts/mattermost.sh waitForStart

restore-keycloak:
	@./scripts/keycloak.sh restore

run: 
	@echo "Starting..."
	@make restore-keycloak
	@make run-core
	@make setup-mattermost
	@make run-ai
	@make run-rtcd
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

nuke: 
	@echo "Nuking Docker..."
	@docker-compose down --volumes --remove-orphans
	@make delete-data

echo-logins:
	@./scripts/general.sh logins