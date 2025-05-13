package cmd

import (
	"fmt"
)

// LoginsCmdF prints login information for various services
func LoginsCmdF() error {
	fmt.Println("===========================================================")
	fmt.Println()
	fmt.Println("- Mattermost: http://localhost:8065 with the logins above if you ran setup")
	fmt.Println("- Keycloak: http://localhost:8080 with 'admin' / 'admin'")
	fmt.Println("- Grafana: http://localhost:3000 with 'admin' / 'admin'")
	fmt.Println("    - All Mattermost Grafana charts are setup.")
	fmt.Println("    - For more info https://github.com/coltoneshaw/mattermost#use-grafana")
	fmt.Println("- Prometheus: http://localhost:9090")
	fmt.Println("- PostgreSQL  localhost:5432 with 'mmuser' / 'mmuser_password'")
	fmt.Println()
	fmt.Println("===========================================================")
	return nil
}
