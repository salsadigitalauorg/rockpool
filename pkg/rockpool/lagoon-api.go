package rockpool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

func (r *Rockpool) lagoonFetchApiToken() string {
	fmt.Println("fetching lagoon api token...")
	_, password := r.KubeGetSecret(r.ControllerClusterName(),
		"lagoon-core",
		"lagoon-core-keycloak",
		"KEYCLOAK_LAGOON_ADMIN_PASSWORD",
	)

	data := url.Values{
		"client_id":  {"lagoon-ui"},
		"grant_type": {"password"},
		"username":   {"lagoonadmin"},
		"password":   {password},
	}
	url := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token", r.State.KeycloakUrl, "lagoon")
	resp, err := http.PostForm(url, data)
	if err != nil {
		fmt.Println("error fetching Lagoon API token:", err)
		os.Exit(1)
	}

	var res struct {
		Token string `json:"access_token"`
	}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		fmt.Println("error parsing Lagoon API token:", err)
		os.Exit(1)
	}
	return res.Token
}

func (r *Rockpool) GetLagoonApiClient() {
	if r.GqlClient != nil {
		return
	}
	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: r.lagoonFetchApiToken()})
	httpClient := oauth2.NewClient(context.Background(), src)
	r.GqlClient = graphql.NewClient(fmt.Sprintf("http://api.%s/graphql", r.Config.LagoonBaseUrl), httpClient)
}

func (r *Rockpool) LagoonApiGetRemotes() {
	fmt.Println("fetching lagoon api remotes...")
	var query struct {
		AllKubernetes []Remote
	}
	err := r.GqlClient.Query(context.Background(), &query, nil)
	if err != nil {
		fmt.Println("error fetching Lagoon remotes:", err)
		os.Exit(1)
	}
	r.State.Remotes = query.AllKubernetes
}

func (r *Rockpool) LagoonApiAddRemote(re Remote, token string) {
	var m struct {
		AddKubernetes struct {
			Remote
		} `graphql:"addKubernetes(input: {id: $id, name: $name, consoleUrl: $console, token: $token, routerPattern: $routePattern})"`
	}
	vars := map[string]interface{}{
		"id":           graphql.Int(re.Id),
		"name":         graphql.String(re.Name),
		"console":      graphql.String(re.ConsoleUrl),
		"token":        graphql.String(token),
		"routePattern": graphql.String(re.RouterPattern),
	}
	err := r.GqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		fmt.Printf("error adding Lagoon remote %s: %#v\n", re.Name, err)
		os.Exit(1)
	}
	r.Remotes = append(r.Remotes, m.AddKubernetes.Remote)
}
