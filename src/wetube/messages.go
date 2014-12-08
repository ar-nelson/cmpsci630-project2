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
	Id      int32
}

type BrowserDisconnect struct{}

type SessionInit struct {
	Name   string
	Leader bool
}

type SessionOk struct{}

type Invitation struct {
	Leader           *RosterEntry
	RankOffered      Rank
	CurrentVideoName string
	Random           int32
}

type InvitationResponse struct {
	Accepted  bool
	Id        int32
	Name      string
	PublicKey *SerializedPublicKey
	Random    int32
}

type JoinConfirmation struct {
	Success bool
	Reason  string
	Roster  []*RosterEntry
}

type Heartbeat struct {
	Random int32
}

type HeartbeatAck struct {
	Random int32
}

type VideoUpdateRequest VideoInstant

type RankChangeRequest struct {
	PeerId  int32
	NewRank Rank
}

type InvitationRequest struct {
	Address    string
	Port       uint16
	Invitation Invitation
}

type RosterUpdate struct {
	Roster []*RosterEntry
}

type RosterEntry struct {
	Id        int32
	Name      string
	Address   string
	Port      uint16
	Rank      Rank
	PublicKey *SerializedPublicKey
}

type SerializedPublicKey struct {
	N []byte
	E int
}

type VideoUpdate VideoInstant

type EndSession struct {
	LeaderId int32
}

type Error struct {
	Message string
}

func (err *Error) Error() string {
	return err.Message
}
