package lagoon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/salsadigitalauorg/rockpool/pkg/interceptor"
	"github.com/salsadigitalauorg/rockpool/pkg/kube"
	"github.com/salsadigitalauorg/rockpool/pkg/platform"
	"github.com/salsadigitalauorg/rockpool/pkg/ssh"
	"github.com/shurcooL/graphql"
	"golang.org/x/oauth2"
)

// Version is the version of Lagoon to be installed.
var Version string

var GqlClient *graphql.Client

var Remotes []Remote

var lagoonUserinfo struct {
	Me struct {
		Id      graphql.String
		Email   graphql.String
		SshKeys []struct {
			Name           string
			KeyFingerprint string
		}
	}
}

// FetchApiAdminToken creates an admin token with superpowers.
// See https://docs.lagoon.sh/administering-lagoon/graphql-queries/#running-graphql-queries
func FetchApiAdminToken() string {
	fmt.Println("fetching lagoon api admin token")
	out, err := kube.Exec(
		platform.ControllerClusterName(), "lagoon-core",
		"lagoon-core-storage-calculator", "/create_jwt.py").Output()
	if err != nil {
		panic(err)
	}
	return string(out)
}

func FetchApiToken() string {
	fmt.Println("fetching lagoon api token")
	_, password := kube.GetSecret(platform.ControllerClusterName(),
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
	url := fmt.Sprintf("http://keycloak.lagoon.%s/auth/realms/lagoon/protocol/openid-connect/token", platform.Hostname())
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(data.Encode()))
	if err != nil {
		panic(err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	dump, _ := httputil.DumpRequest(req, true)

	client := &http.Client{
		Transport: interceptor.New(),
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("request:", string(dump))
		panic(err)
	}
	dump, _ = httputil.DumpResponse(resp, true)

	var res struct {
		Token            string `json:"access_token"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		fmt.Println("response:", string(dump))
		fmt.Println("error parsing Lagoon API token:", err)
		os.Exit(1)
	}
	if res.Error != "" {
		fmt.Printf("error fetching Lagoon API token: %s - %s\n", res.Error, res.ErrorDescription)
		os.Exit(1)
	}
	return res.Token
}

func InitApiClient() {
	if GqlClient != nil {
		return
	}
	src := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: FetchApiToken()})
	httpClient := &http.Client{
		Transport: &oauth2.Transport{
			Base:   interceptor.New(),
			Source: oauth2.ReuseTokenSource(nil, src),
		},
	}
	GqlClient = graphql.NewClient(fmt.Sprintf("http://api.lagoon.%s/graphql", platform.Hostname()), httpClient)
}

func GetRemotes() {
	fmt.Println("fetching lagoon api remotes...")
	var query struct {
		AllKubernetes []Remote
	}
	err := GqlClient.Query(context.Background(), &query, nil)
	if err != nil {
		fmt.Println("error fetching Lagoon remotes:", err)
		os.Exit(1)
	}
	Remotes = query.AllKubernetes
}

func FetchUserInfo() {
	err := GqlClient.Query(context.Background(), &lagoonUserinfo, nil)
	if err != nil {
		fmt.Println("error fetching Lagoon user info:", err)
		os.Exit(1)
	}
}

func AddSshKey() {
	keyValue, keyType, keyFingerpint, cmt := ssh.GetPublicKeyFingerprint()
	FetchUserInfo()
	for _, k := range lagoonUserinfo.Me.SshKeys {
		if k.KeyFingerprint == keyFingerpint {
			fmt.Println("lagoon ssh key had already been added")
			return
		}
	}

	var m struct {
		AddSshKey struct {
			Name           string
			KeyFingerprint string
		} `graphql:"addSshKey(input: {keyType: $keyType, keyValue: $keyValue, name: $name, user: {email: $userEmail, id: $userId}})"`
	}

	type SshKeyType string
	vars := map[string]interface{}{
		"keyType":   SshKeyType(strings.ReplaceAll(strings.ToUpper(keyType), "-", "_")),
		"keyValue":  graphql.String(keyValue),
		"name":      graphql.String(cmt),
		"userEmail": graphql.String(lagoonUserinfo.Me.Email),
		"userId":    graphql.String(lagoonUserinfo.Me.Id),
	}
	fmt.Println("adding lagoon ssh key")
	err := GqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		fmt.Printf("error adding Lagoon ssh key %s: %#v\n", cmt, err)
		os.Exit(1)
	}
}

func AddRemote(re Remote, token string) {
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
	err := GqlClient.Mutate(context.Background(), &m, vars)
	if err != nil {
		fmt.Printf("error adding Lagoon remote %s: %#v\n", re.Name, err)
		os.Exit(1)
	}
	Remotes = append(Remotes, m.AddKubernetes.Remote)
}
