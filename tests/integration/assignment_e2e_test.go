package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssignmentWorkflowE2E(t *testing.T) {
	skipIfServicesDown(t)
	pool := testPool(t)
	ctx := context.Background()

	adminEmail := uniqueEmail("assign-flow-admin")
	adminClient := newClient()

	// 1. Sign up admin
	t.Log("E2E Step 1: Sign up Admin")
	resp, body := post(t, adminClient, gatewayURL+"/auth/signup", map[string]string{
		"account_name": "E2E Assignment Test Account",
		"email":        adminEmail,
		"password":     "AdminPassword123!",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode, "admin signup must return 201: %v", body)
	accountIDStr := body["account_id"].(string)
	accountID := uuid.MustParse(accountIDStr)

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM sessions WHERE data::text LIKE '%'||$1||'%'`, accountIDStr)
		pool.Exec(ctx, `DELETE FROM messages WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM conversations WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM contacts WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM channels WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM audit_logs WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM users WHERE account_id = $1`, accountID)
		pool.Exec(ctx, `DELETE FROM accounts WHERE id = $1`, accountID)
	})

	// Log in Admin
	t.Log("E2E Step 2: Log in Admin")
	loginResp, _ := post(t, adminClient, gatewayURL+"/auth/login", map[string]string{
		"email":    adminEmail,
		"password": "AdminPassword123!",
	})
	require.Equal(t, http.StatusOK, loginResp.StatusCode, "admin login must succeed")

	// 2. Create 2 agent users (Agent A and Agent B)
	t.Log("E2E Step 3: Create 2 agent users")
	createRespA, createBodyA := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
		"username": "agent_alpha",
		"email":    uniqueEmail("agent_alpha"),
		"password": "AgentPassword123!",
		"role":     "agent",
	})
	require.Equal(t, http.StatusCreated, createRespA.StatusCode, "agent A creation failed: %v", createBodyA)
	agentAUserID := createBodyA["id"].(string)

	createRespB, createBodyB := post(t, adminClient, gatewayURL+"/workspace/users", map[string]string{
		"username": "agent_beta",
		"email":    uniqueEmail("agent_beta"),
		"password": "AgentPassword123!",
		"role":     "agent",
	})
	require.Equal(t, http.StatusCreated, createRespB.StatusCode, "agent B creation failed: %v", createBodyB)
	agentBUserID := createBodyB["id"].(string)

	// Log in Agent A and Agent B clients
	agentAClient := newClient()
	loginRespA, _ := post(t, agentAClient, gatewayURL+"/auth/login", map[string]string{
		"email":    createBodyA["email"].(string),
		"password": "AgentPassword123!",
	})
	require.Equal(t, http.StatusOK, loginRespA.StatusCode)

	agentBClient := newClient()
	loginRespB, _ := post(t, agentBClient, gatewayURL+"/auth/login", map[string]string{
		"email":    createBodyB["email"].(string),
		"password": "AgentPassword123!",
	})
	require.Equal(t, http.StatusOK, loginRespB.StatusCode)

	// 3. Create Channel, Contact, and Conversation
	t.Log("E2E Step 4: Create channel, contact, and conversation")
	var channelID, contactID, convoID uuid.UUID
	err := pool.QueryRow(ctx, `INSERT INTO channels (account_id, type, status) VALUES ($1, 'matrix_whatsapp', 'connected') RETURNING id`, accountID).Scan(&channelID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `INSERT INTO contacts (account_id, channel_id, external_identity) VALUES ($1, $2, 'contact-e2e-assign') RETURNING id`, accountID, channelID).Scan(&contactID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx, `INSERT INTO conversations (account_id, contact_id, channel_id, status, assigned_user_ids) VALUES ($1, $2, $3, 'open', '{}') RETURNING id`, accountID, contactID, channelID).Scan(&convoID)
	require.NoError(t, err)
	convoIDStr := convoID.String()

	// 4. Single Assignment: Assign to Agent A
	t.Log("E2E Step 5: Assign conversation to Agent A")
	assignResp, assignBody := patch(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/assign", map[string]any{
		"user_ids": []string{agentAUserID},
	})
	require.Equal(t, http.StatusOK, assignResp.StatusCode, "single assign failed: %v", assignBody)

	// Verify conversation GET returns Agent A
	getResp, getBody := get(t, adminClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	assignedList, ok := getBody["assigned_user_ids"].([]any)
	require.True(t, ok)
	require.Len(t, assignedList, 1)
	assert.Equal(t, agentAUserID, assignedList[0].(string))

	// 5. Multi-Assignment: Assign to Agent A AND Agent B
	t.Log("E2E Step 6: Multi-assign conversation to Agent A and Agent B")
	multiAssignResp, multiAssignBody := patch(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/assign", map[string]any{
		"user_ids": []string{agentAUserID, agentBUserID},
	})
	require.Equal(t, http.StatusOK, multiAssignResp.StatusCode, "multi-assign failed: %v", multiAssignBody)

	getResp2, getBody2 := get(t, adminClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusOK, getResp2.StatusCode)
	assignedList2, ok := getBody2["assigned_user_ids"].([]any)
	require.True(t, ok)
	require.Len(t, assignedList2, 2)
	assignedStrings := []string{assignedList2[0].(string), assignedList2[1].(string)}
	assert.Contains(t, assignedStrings, agentAUserID)
	assert.Contains(t, assignedStrings, agentBUserID)

	// Verify filter=mine returns the conversation for both agents
	listRespA, listBodyA := get(t, agentAClient, gatewayURL+"/conversations?filter=mine")
	require.Equal(t, http.StatusOK, listRespA.StatusCode)
	convoArrayA, _ := listBodyA["conversations"].([]any)
	assert.NotEmpty(t, convoArrayA)

	listRespB, listBodyB := get(t, agentBClient, gatewayURL+"/conversations?filter=mine")
	require.Equal(t, http.StatusOK, listRespB.StatusCode)
	convoArrayB, _ := listBodyB["conversations"].([]any)
	assert.NotEmpty(t, convoArrayB)

	// 6. Explicit Unassignment: Clear assignees
	t.Log("E2E Step 7: Unassign conversation completely")
	unassignResp, unassignBody := patch(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/assign", map[string]any{
		"user_ids": []string{},
	})
	require.Equal(t, http.StatusOK, unassignResp.StatusCode, "unassign failed: %v", unassignBody)

	getResp3, getBody3 := get(t, adminClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusOK, getResp3.StatusCode)
	assignedList3, _ := getBody3["assigned_user_ids"].([]any)
	assert.Empty(t, assignedList3)

	// 7. Delete Agent A: Verify deleting user cleans up any assignments
	t.Log("E2E Step 8: Reassign to Agent A and delete Agent A")
	_, _ = patch(t, adminClient, gatewayURL+"/conversations/"+convoIDStr+"/assign", map[string]any{
		"user_ids": []string{agentAUserID},
	})

	// Delete Agent A
	delResp, delBody := deleteReq(t, adminClient, gatewayURL+"/workspace/users/"+agentAUserID)
	require.Equal(t, http.StatusOK, delResp.StatusCode, "delete user failed: %v", delBody)

	// Verify conversation assigned_user_ids was cleaned up
	getResp4, getBody4 := get(t, adminClient, gatewayURL+"/conversations/"+convoIDStr)
	require.Equal(t, http.StatusOK, getResp4.StatusCode)
	assignedList4, _ := getBody4["assigned_user_ids"].([]any)
	assert.Empty(t, assignedList4)
}

func deleteReq(t *testing.T, client *http.Client, urlStr string) (*http.Response, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, urlStr, nil)
	require.NoError(t, err)
	attachCSRF(req, client)
	resp, err := client.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result) //nolint:errcheck
	return resp, result
}

