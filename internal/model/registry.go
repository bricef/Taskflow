package model

// Named resources — exported for direct use by httpclient consumers.
var (
	ResActorList   = mustRes("actor_list")
	ResActorGet    = mustRes("actor_get")
	ResBoardList   = mustRes("board_list")
	ResBoardGet    = mustRes("board_get")
	ResWorkflowGet = mustRes("workflow_get")
	ResTaskList    = mustRes("task_list")
	ResTaskGet     = mustRes("task_get")
	ResTaskDetail  = mustRes("task_detail")
	ResTagList     = mustRes("tag_list")
	ResCommentList = mustRes("comment_list")
	ResDepList     = mustRes("dependency_list")
	ResAttachList  = mustRes("attachment_list")
	ResTaskSearch  = mustRes("task_search")
	ResBoardDetail = mustRes("board_detail")
	ResBoardOver   = mustRes("board_overview")
	ResAdminStats  = mustRes("admin_stats")
	ResWebhookList = mustRes("webhook_list")
	ResWebhookGet  = mustRes("webhook_get")
	ResDeliveries  = mustRes("delivery_list")
)

// Named operations — exported for direct use by httpclient consumers.
var (
	OpActorCreate    = mustOp("actor_create")
	OpActorUpdate    = mustOp("actor_update")
	OpBoardCreate    = mustOp("board_create")
	OpBoardUpdate    = mustOp("board_update")
	OpBoardDelete    = mustOp("board_delete")
	OpBoardReassign  = mustOp("board_reassign")
	OpWorkflowSet    = mustOp("workflow_set")
	OpWorkflowHealth = mustOp("workflow_health")
	OpTaskCreate     = mustOp("task_create")
	OpTaskUpdate     = mustOp("task_update")
	OpTaskTransition = mustOp("task_transition")
	OpTaskDelete     = mustOp("task_delete")
	OpTaskAudit      = mustOp("task_audit")
	OpBoardAudit     = mustOp("board_audit")
	OpCommentCreate  = mustOp("comment_create")
	OpCommentUpdate  = mustOp("comment_update")
	OpDepCreate      = mustOp("dependency_create")
	OpDepDelete      = mustOp("dependency_delete")
	OpAttachCreate   = mustOp("attachment_create")
	OpAttachDelete   = mustOp("attachment_delete")
	OpWebhookCreate  = mustOp("webhook_create")
	OpWebhookUpdate  = mustOp("webhook_update")
	OpWebhookDelete  = mustOp("webhook_delete")
)

func mustRes(name string) Resource {
	r, ok := LookupResource(name)
	if !ok {
		panic("unknown resource: " + name)
	}
	return r
}

func mustOp(name string) Operation {
	op, ok := LookupOperation(name)
	if !ok {
		panic("unknown operation: " + name)
	}
	return op
}
