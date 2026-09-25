package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"satellite-contact-window-deconfliction/backend/internal/dto"
	"satellite-contact-window-deconfliction/backend/internal/service"
)

type ContactBackfillHandler struct {
	service *service.ContactBackfillService
}

func NewContactBackfillHandler(service *service.ContactBackfillService) *ContactBackfillHandler {
	return &ContactBackfillHandler{service: service}
}

func (handler *ContactBackfillHandler) List(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	backfills, err := handler.service.List(id)
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusOK, backfills)
}

func (handler *ContactBackfillHandler) Create(context *gin.Context) {
	id, err := PathID(context)
	if err != nil {
		WriteError(context, err)
		return
	}
	var request dto.CreateBackfillRequest
	if err := BindAndValidate(context, &request); err != nil {
		WriteError(context, err)
		return
	}
	backfill, err := handler.service.Create(id, request, Actor(context), RequestID(context))
	if err != nil {
		WriteError(context, err)
		return
	}
	WriteData(context, http.StatusCreated, backfill)
}
