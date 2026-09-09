package admin

const (
	CodeParamError     = 10000
	CodeBadJSON        = 10001
	CodeNotFound       = 10002
	CodeConflict       = 10003
	CodeForbiddenState = 10004
	CodeInternal       = 10005
	CodeBodyTooLarge   = 10006
	CodeNotImplemented = 99999
)

const (
	CodeChNotFound        = 20000
	CodeChDisabled        = 20001
	CodeChModelDuplicate  = 20002
	CodeChModelNotFound   = 20003
	CodeChRequiredMissing = 20004
	CodeChSecretMissing   = 20006
)

const (
	CodeUserNotFound      = 10100
	CodeUserStateOp       = 10101
	CodeUserAmountInvalid = 10102
	CodeKeyNotFound       = 10200
	CodeKeyNameDuplicate  = 10201
	CodeKeyPrefixInvalid  = 10202
	CodeUserSuspended     = 10203
)
