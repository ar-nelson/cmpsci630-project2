package wetube

type MsgType string

const (
	T_BrowserConnectAck  MsgType = "BrowserConnectAck"
	T_BrowserDisconnect          = "BrowserDisconnect"
	T_SessionInit                = "SessionInit"
	T_SessionOk                  = "SessionOk"
	T_Invitation                 = "Invitation"
	T_InvitationResponse         = "InvitationResponse"
	T_JoinConfirmation           = "JoinConfirmation"
	T_Heartbeat                  = "Heartbeat"
	T_HeartbeatAck               = "HeartbeatAck"
	T_VideoUpdateRequest         = "VideoUpdateRequest"
	T_RankChangeRequest          = "RankChangeRequest"
	T_InvitationRequest          = "InvitationRequest"
	T_RosterUpdate               = "RosterUpdate"
	T_VideoUpdate                = "VideoUpdate"
	T_EndSession                 = "EndSession"
	T_Error                      = "Error"
)

type BrowserConnectAck struct {
	Success bool
	Reason  string
}

type BrowserDisconnect struct{}

type SessionInit struct {
	Name   string
	Leader bool
}

type SessionOk struct{}

type Invitation struct {
	Leader           RosterEntry
	RankOffered      Rank
	CurrentVideoName string
}

type InvitationResponse struct {
	Accepted bool
	PeerData *JoinResponseData
}

type JoinResponseData struct {
	Id   string
	Name string
}

type JoinConfirmation struct {
	Success bool
	Reason  string
	Roster  RosterUpdate
}

type Heartbeat struct {
	Random int
}

type HeartbeatAck struct {
	Random int
}

type VideoUpdateRequest Video

type RankChangeRequest struct {
	PeerId  int
	NewRank Rank
}

type InvitationRequest struct {
	Address    string
	Invitation Invitation
}

type RosterUpdate struct {
	Leader *RosterEntry
	Others []RosterEntry
}

type RosterEntry struct {
	Id        int
	Name      string
	Address   string
	Rank      Rank
	PublicKey []byte
}

type VideoUpdate Video

type EndSession struct {
	LeaderId int
}

type Error struct {
	Message string
}

func (err *Error) Error() string {
	return err.Message
}
