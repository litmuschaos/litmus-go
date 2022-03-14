package connections

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/litmuschaos/litmus-go/pkg/machine/common/messages"
	"github.com/litmuschaos/litmus-go/pkg/types"
	"github.com/pkg/errors"
)

// CreateWebsocketConnections creates all the requisite websocket connections from the given agent endpoints and authentication tokens.
// It also initialises a listener goroutine for every connection.
func CreateWebsocketConnections(agentEndpoints, authTokens string, connectMultipleAgents bool, chaosDetails *types.ChaosDetails) error {

	if agentEndpoints == "" {
		return errors.Errorf("no agent endpoint found")
	}

	if authTokens == "" {
		return errors.Errorf("no authentication token found")
	}

	agentEndpointList := strings.Split(agentEndpoints, ",")
	authTokenList := strings.Split(authTokens, ",")

	if len(agentEndpointList) != len(authTokenList) {
		return errors.Errorf("unequal number of agent endpoints and authentication tokens found")
	}

	if !connectMultipleAgents && (len(agentEndpointList) > 1) {
		return errors.Errorf("multiple agent endpoints received, please input only one endpoint and corressponding authentication token")
	}

	chaosDetails.WebsocketConnections = []*websocket.Conn{}

	for i := range agentEndpointList {

		conn, _, err := websocket.DefaultDialer.Dial("ws://"+agentEndpointList[i]+"/"+chaosDetails.ExperimentName, http.Header{"Authorization": []string{"Bearer " + authTokenList[i]}})
		if err != nil {
			return err
		}

		go messages.ListenForAgentMessage(conn)

		chaosDetails.WebsocketConnections = append(chaosDetails.WebsocketConnections, conn)
	}

	return nil
}

// CloseWebsocketConnections performs the graceful closure of the websocket connections
func CloseWebsocketConnections(connections []*websocket.Conn) error {

	for _, conn := range connections {

		_, _, err := messages.SendMessageToAgent(conn, "CLOSE_CONNECTION", nil, nil)
		if err != nil {
			return err
		}
	}

	return nil
}
