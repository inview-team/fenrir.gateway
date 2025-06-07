package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"chatops-bot/internal/models"
	"chatops-bot/internal/service"

	"gopkg.in/telebot.v3"
)

// Callback Prefixes
const (
	viewIncidentPrefix          = "vi:"
	showActionsPrefix           = "sa:"
	performActionPrefix         = "pa:"
	closeIncidentPrefix         = "ci:"
	setStatusPrefix             = "ss:"
	viewResourcePrefix          = "vr:"
	performResourceActionPrefix = "pra:"
	scaleDeploymentPrefix       = "scd:"
	allocateHardwarePrefix      = "ahw:"
)

// State for awaiting user input
type awaitingInputState struct {
	Request   *models.ActionRequest
	MessageID int
	ChatID    int64
}

type userState struct {
	AwaitingRejectReasonFor    uint
	AwaitingReplicaCountFor    *awaitingInputState
	AwaitingHardwareRequestFor *awaitingInputState
}

type Bot struct {
	bot        *telebot.Bot
	service    *service.IncidentService
	userRepo   service.UserRepository
	suggester  *service.ActionSuggester
	userStates map[int64]*userState
	mu         sync.RWMutex
}

func NewBot(token string, service *service.IncidentService, userRepo service.UserRepository, suggester *service.ActionSuggester) (*Bot, error) {
	pref := telebot.Settings{Token: token, Poller: &telebot.LongPoller{Timeout: 10 * time.Second}}
	b, err := telebot.NewBot(pref)
	if err != nil {
		return nil, err
	}
	botInstance := &Bot{
		bot:        b,
		service:    service,
		userRepo:   userRepo,
		suggester:  suggester,
		userStates: make(map[int64]*userState),
	}
	b.Use(botInstance.authMiddleware())
	return botInstance, nil
}

func (b *Bot) Start(notifChan <-chan *models.Incident) {
	b.registerHandlers()
	go b.startNotifier(notifChan)
	log.Println("Telegram bot starting...")
	b.bot.Start()
}

func (b *Bot) startNotifier(notifChan <-chan *models.Incident) {
	log.Println("Notification listener started.")
	for incident := range notifChan {
		log.Printf("Received notification for new incident: %s", incident.Summary)
		users, err := b.userRepo.ListAll(context.Background())
		if err != nil {
			log.Printf("Error getting users for notification: %v", err)
			continue
		}
		message := fmt.Sprintf("🚨 *Новый инцидент: %s*\n\n*Описание:* %s", escapeMarkdown(incident.Summary), escapeMarkdown(incident.Description))
		keyboard := &telebot.ReplyMarkup{
			InlineKeyboard: [][]telebot.InlineButton{
				{{Text: "Посмотреть действия", Data: showActionsPrefix + strconv.FormatUint(uint64(incident.ID), 10)}},
			},
		}
		for _, user := range users {
			_, err := b.bot.Send(&telebot.User{ID: user.TelegramID}, message, keyboard, telebot.ModeMarkdownV2)
			if err != nil {
				log.Printf("Failed to send notification to user %d: %v", user.TelegramID, err)
			}
		}
	}
}

func (b *Bot) registerHandlers() {
	b.bot.Handle("/start", b.handleStart)
	b.bot.Handle("/incidents", b.handleListIncidents)
	b.bot.Handle("/history", b.handleHistory)
	b.bot.Handle(telebot.OnCallback, b.handleCallback)
	b.bot.Handle(telebot.OnText, b.handleTextMessage)
}

func (b *Bot) handleStart(c telebot.Context) error {
	return c.Send("Добро пожаловать! Используйте /incidents для просмотра активных инцидентов и /history для просмотра закрытых.")
}

func (b *Bot) handleListIncidents(c telebot.Context) error {
	incidents, err := b.service.ListActiveIncidents(c.Get("ctx").(context.Context))
	if err != nil {
		return c.Send("Не удалось получить список инцидентов.")
	}
	if len(incidents) == 0 {
		return c.Send("Активных инцидентов нет.")
	}
	var keyboard [][]telebot.InlineButton
	for _, inc := range incidents {
		row := []telebot.InlineButton{{
			Text: fmt.Sprintf("🚨 %s (%s)", inc.Summary, inc.Status),
			Data: showActionsPrefix + strconv.FormatUint(uint64(inc.ID), 10),
		}}
		keyboard = append(keyboard, row)
	}
	return c.Send("Активные инциденты:", &telebot.ReplyMarkup{InlineKeyboard: keyboard})
}

func (b *Bot) handleHistory(c telebot.Context) error {
	incidents, err := b.service.ListClosed(c.Get("ctx").(context.Context), 10, 0)
	if err != nil {
		return c.Send("Не удалось получить историю инцидентов.")
	}
	if len(incidents) == 0 {
		return c.Send("История закрытых инцидентов пуста.")
	}
	var keyboard [][]telebot.InlineButton
	for _, inc := range incidents {
		icon := "✅"
		if inc.Status == models.StatusRejected {
			icon = "❌"
		}
		row := []telebot.InlineButton{{
			Text: fmt.Sprintf("%s %s (%s)", icon, inc.Summary, inc.Status),
			Data: viewIncidentPrefix + strconv.FormatUint(uint64(inc.ID), 10),
		}}
		keyboard = append(keyboard, row)
	}
	return c.Send("Последние закрытые инциденты:", &telebot.ReplyMarkup{InlineKeyboard: keyboard})
}

func (b *Bot) handleCallback(c telebot.Context) error {
	data := c.Data()
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return c.Respond()
	}
	prefix := parts[0] + ":"
	incidentID, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid incident ID"})
	}

	switch prefix {
	case viewIncidentPrefix:
		return b.showIncidentView(c, uint(incidentID))
	case showActionsPrefix:
		return b.showActionsView(c, uint(incidentID))
	case closeIncidentPrefix:
		return b.showCloseOptions(c, uint(incidentID))
	case setStatusPrefix:
		return b.handleSetStatus(c)
	case performActionPrefix:
		return b.handlePerformAction(c)
	case viewResourcePrefix:
		return b.showResourceActionsView(c)
	case performResourceActionPrefix:
		return b.handlePerformResourceAction(c)
	case scaleDeploymentPrefix:
		return b.handleScaleDeployment(c)
	case allocateHardwarePrefix:
		return b.handleAllocateHardware(c)
	default:
		return c.Respond()
	}
}

func (b *Bot) handleTextMessage(c telebot.Context) error {
	b.mu.Lock()
	state, ok := b.userStates[c.Sender().ID]
	if !ok {
		b.mu.Unlock()
		return nil
	}

	if state.AwaitingRejectReasonFor != 0 {
		incidentID := state.AwaitingRejectReasonFor
		state.AwaitingRejectReasonFor = 0
		b.mu.Unlock()

		reason := c.Text()
		user := c.Get("ctx").(context.Context).Value("user").(*models.User)

		err := b.service.UpdateStatus(c.Get("ctx").(context.Context), user.ID, incidentID, models.StatusRejected, reason)
		if err != nil {
			return c.Send("Не удалось обновить статус инцидента.")
		}
		c.Send("Инцидент отклонен. Спасибо за обратную связь!")
		return c.Delete()
	}

	if state.AwaitingReplicaCountFor != nil {
		inputState := state.AwaitingReplicaCountFor
		state.AwaitingReplicaCountFor = nil
		b.mu.Unlock()

		replicaCount, err := strconv.Atoi(c.Text())
		if err != nil || replicaCount < 0 {
			return c.Send("Неверное количество реплик. Пожалуйста, введите целое положительное число.")
		}

		req := inputState.Request
		req.Parameters["replicas"] = c.Text()
		result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), *req)
		if err != nil {
			c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("Ошибка: %v", err)})
		} else {
			c.Respond(&telebot.CallbackResponse{Text: result.Message, ShowAlert: true})
		}

		c.Delete()
		return b.renderResourceActionsView(c, req.IncidentID, "deployment", req.Parameters["deployment"], &inputState.ChatID, &inputState.MessageID)
	}

	if state.AwaitingHardwareRequestFor != nil {
		inputState := state.AwaitingHardwareRequestFor
		state.AwaitingHardwareRequestFor = nil
		b.mu.Unlock()

		req := inputState.Request
		req.Parameters["resources"] = c.Text()
		result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), *req)
		if err != nil {
			c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("Ошибка: %v", err)})
		} else {
			c.Respond(&telebot.CallbackResponse{Text: result.Message, ShowAlert: true})
		}

		c.Delete()
		return b.renderResourceActionsView(c, req.IncidentID, "pod", req.Parameters["pod"], &inputState.ChatID, &inputState.MessageID)
	}

	b.mu.Unlock()
	return nil
}

func (b *Bot) showIncidentView(c telebot.Context, incidentID uint) error {
	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}
	message := b.formatIncidentMessage(incident)
	keyboard := b.buildIncidentViewKeyboard(incident)
	err = c.Edit(message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return c.Respond()
	}
	return err
}

func (b *Bot) showActionsView(c telebot.Context, incidentID uint) error {
	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}
	message := b.formatIncidentMessage(incident)
	suggestedActions := b.suggester.SuggestActions(incident)
	keyboard := b.buildActionsViewKeyboard(incident, suggestedActions)
	err = c.Edit(message, &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return c.Respond()
	}
	return err
}

func (b *Bot) renderResourceActionsView(c telebot.Context, incidentID uint, resourceType, resourceName string, chatID *int64, messageID *int) error {
	ctx := c.Get("ctx").(context.Context)
	incident, err := b.service.GetIncidentByID(ctx, incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}

	detailsReq := models.ResourceDetailsRequest{
		IncidentID:   incidentID,
		ResourceType: resourceType,
		ResourceName: resourceName,
		Labels:       incident.Labels,
	}
	details, err := b.service.GetResourceDetails(ctx, detailsReq)

	var messageBuilder strings.Builder
	messageBuilder.WriteString(fmt.Sprintf("*Ресурс: %s `%s`*\n\n", strings.Title(resourceType), escapeMarkdown(resourceName)))

	if err != nil {
		log.Printf("Could not get resource details: %v", err)
		messageBuilder.WriteString("_Не удалось загрузить детали ресурса\\._\n\n")
	} else {
		messageBuilder.WriteString(fmt.Sprintf("∙ *Статус:* `%s`\n", escapeMarkdown(details.Status)))
		if details.ReplicasInfo != "" {
			messageBuilder.WriteString(fmt.Sprintf("∙ *Реплики:* `%s`\n", escapeMarkdown(details.ReplicasInfo)))
		}
		if details.Restarts != "" {
			messageBuilder.WriteString(fmt.Sprintf("∙ *Перезапуски:* `%s`\n", escapeMarkdown(details.Restarts)))
		}
		messageBuilder.WriteString(fmt.Sprintf("∙ *Возраст:* `%s`\n", escapeMarkdown(details.Age)))
		messageBuilder.WriteString("\n")
	}

	messageBuilder.WriteString("Выберите действие:")

	actions := b.suggester.SuggestActionsForResource(incident, resourceType, resourceName)
	keyboard := b.buildResourceActionsKeyboard(incident, resourceType, resourceName, actions)

	messageText := messageBuilder.String()
	replyMarkup := &telebot.ReplyMarkup{InlineKeyboard: keyboard}

	if messageID != nil && chatID != nil {
		editable := &telebot.StoredMessage{MessageID: strconv.Itoa(*messageID), ChatID: *chatID}
		_, err = b.bot.Edit(editable, messageText, replyMarkup, telebot.ModeMarkdownV2)
	} else {
		err = c.Edit(messageText, replyMarkup, telebot.ModeMarkdownV2)
	}

	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return c.Respond()
	}
	return err
}

func (b *Bot) showResourceActionsView(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	if len(parts) < 4 {
		log.Printf("Invalid callback data for showResourceActionsView: %s", c.Data())
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid callback data"})
	}
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceType := parts[2]
	resourceName := parts[3]

	return b.renderResourceActionsView(c, uint(incidentID), resourceType, resourceName, nil, nil)
}

func (b *Bot) showCloseOptions(c telebot.Context, incidentID uint) error {
	keyboard := b.buildCloseOptionsKeyboard(incidentID)
	return c.Edit("Выберите статус для закрытия инцидента:", &telebot.ReplyMarkup{InlineKeyboard: keyboard})
}

func (b *Bot) handleSetStatus(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	status := models.IncidentStatus(parts[2])
	user := c.Get("ctx").(context.Context).Value("user").(*models.User)

	if status == models.StatusRejected {
		b.mu.Lock()
		b.userStates[c.Sender().ID] = &userState{AwaitingRejectReasonFor: uint(incidentID)}
		b.mu.Unlock()
		return c.Edit("Пожалуйста, введите причину отклонения инцидента одним сообщением.")
	}

	err := b.service.UpdateStatus(c.Get("ctx").(context.Context), user.ID, uint(incidentID), status, "")
	if err != nil {
		return c.Send("Не удалось обновить статус инцидента.")
	}
	c.Send(fmt.Sprintf("Статус инцидента обновлен на '%s'.", status))
	return c.Delete()
}

func (b *Bot) handlePerformAction(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	actionIndex, err := strconv.Atoi(parts[2])
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid action index"})
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Incident not found"})
	}

	actions := b.suggester.SuggestActions(incident)
	if actionIndex < 0 || actionIndex >= len(actions) {
		return c.Respond(&telebot.CallbackResponse{Text: "Action no longer valid"})
	}
	action := actions[actionIndex]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)
	req := models.ActionRequest{
		Action:     action.Action,
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: action.Parameters,
	}

	result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), req)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("Ошибка: %v", err)})
	}

	return b.handleActionResult(c, uint(incidentID), req, result)
}

func (b *Bot) handlePerformResourceAction(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceType := parts[2]
	resourceName := parts[3]
	actionIndex, err := strconv.Atoi(parts[4])
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Invalid action index"})
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), uint(incidentID))
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Incident not found"})
	}

	actions := b.suggester.SuggestActionsForResource(incident, resourceType, resourceName)
	if actionIndex < 0 || actionIndex >= len(actions) {
		return c.Respond(&telebot.CallbackResponse{Text: "Action no longer valid"})
	}
	action := actions[actionIndex]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)
	req := models.ActionRequest{
		Action:     action.Action,
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: action.Parameters,
	}

	result, err := b.service.ExecuteAction(c.Get("ctx").(context.Context), req)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: fmt.Sprintf("Ошибка: %v", err)})
	}

	return b.handleActionResult(c, uint(incidentID), req, result)
}

func (b *Bot) handleActionResult(c telebot.Context, incidentID uint, req models.ActionRequest, result models.ActionResult) error {
	actionType := models.ActionType(req.Action)
	if actionType == models.ActionGetPodLogs || actionType == models.ActionDescribePod || actionType == models.ActionListPodsForDeployment {
		c.Respond()
	} else {
		alertText := result.Message
		if result.Error != "" {
			alertText = result.Error
		}
		c.Respond(&telebot.CallbackResponse{Text: alertText, ShowAlert: true})
	}

	if result.Error != "" {
		return b.showIncidentView(c, incidentID)
	}

	switch actionType {
	case models.ActionGetPodLogs, models.ActionDescribePod, models.ActionDescribeDeployment:
		if len(result.Message) > 4096 {
			result.Message = result.Message[:4090] + "\n..."
		}
		formattedMessage := fmt.Sprintf("```\n%s\n```", result.Message)
		b.bot.Send(c.Chat(), formattedMessage, telebot.ModeMarkdown)
	case models.ActionListPodsForDeployment:
		if result.ResultData != nil && len(result.ResultData.Items) > 0 {
			return b.showDynamicResourceList(c, incidentID, result)
		}
	}

	if req.Action == string(models.ActionScaleDeployment) || req.Action == string(models.ActionAllocateHardware) {
		return nil
	}

	var callbackData string
	if c.Callback() != nil {
		callbackData = c.Callback().Data
	}

	if strings.HasPrefix(callbackData, performResourceActionPrefix) {
		return b.showResourceActionsView(c)
	}

	return b.showActionsView(c, incidentID)
}

func (b *Bot) showDynamicResourceList(c telebot.Context, incidentID uint, result models.ActionResult) error {
	var keyboard [][]telebot.InlineButton
	for _, item := range result.ResultData.Items {
		statusIcon := "🟢"
		if item.Status != "Running" {
			statusIcon = "🔴"
		}
		callbackData := fmt.Sprintf("%s%d:%s:%s", viewResourcePrefix, incidentID, result.ResultData.ItemType, item.Name)
		btn := telebot.InlineButton{Text: fmt.Sprintf("%s %s (%s)", statusIcon, item.Name, item.Status), Data: callbackData}
		keyboard = append(keyboard, []telebot.InlineButton{btn})
	}

	incident, err := b.service.GetIncidentByID(c.Get("ctx").(context.Context), incidentID)
	if err != nil {
		return c.EditOrSend("Не удалось найти инцидент.")
	}

	keyboard = append(keyboard, []telebot.InlineButton{
		{Text: "⬅️ Назад", Data: showActionsPrefix + strconv.FormatUint(uint64(incidentID), 10)},
		{Text: "🏠 К инциденту", Data: viewIncidentPrefix + strconv.FormatUint(uint64(incidentID), 10)},
	})

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incidentID), 10)}})
	}

	return c.Edit(escapeMarkdown(result.Message), &telebot.ReplyMarkup{InlineKeyboard: keyboard}, telebot.ModeMarkdownV2)
}

func (b *Bot) buildIncidentViewKeyboard(incident *models.Incident) [][]telebot.InlineButton {
	if incident.Status != models.StatusActive {
		return nil
	}
	return [][]telebot.InlineButton{
		{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}},
		{{Text: "▶️ Выполнить действия", Data: showActionsPrefix + strconv.FormatUint(uint64(incident.ID), 10)}},
	}
}

func (b *Bot) buildActionsViewKeyboard(incident *models.Incident, actions []models.SuggestedAction) [][]telebot.InlineButton {
	var keyboard [][]telebot.InlineButton
	var actionRow []telebot.InlineButton
	for i, action := range actions {
		callbackData := fmt.Sprintf("%s%d:%d", performActionPrefix, incident.ID, i)
		actionRow = append(actionRow, telebot.InlineButton{Text: action.HumanReadable, Data: callbackData})
	}
	if len(actionRow) > 0 {
		keyboard = append(keyboard, actionRow)
	}

	if len(incident.AffectedResources) > 0 {
		if deployment, ok := incident.AffectedResources["deployment"]; ok {
			callbackData := fmt.Sprintf("%s%d:%s:%s", viewResourcePrefix, incident.ID, "deployment", deployment)
			keyboard = append(keyboard, []telebot.InlineButton{{Text: "🗂️ Действия с Deployment", Data: callbackData}})
		}
	}

	keyboard = append(keyboard, []telebot.InlineButton{{Text: "⬅️ Назад", Data: viewIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}})

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}})
	}

	return keyboard
}

func (b *Bot) buildResourceActionsKeyboard(incident *models.Incident, resourceType, resourceName string, actions []models.SuggestedAction) [][]telebot.InlineButton {
	var keyboard [][]telebot.InlineButton
	incidentID := incident.ID
	for i, action := range actions {
		callbackData := fmt.Sprintf("%s%d:%s:%s:%d", performResourceActionPrefix, incidentID, resourceType, resourceName, i)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: action.HumanReadable, Data: callbackData}})
	}

	if resourceType == "deployment" {
		namespace := incident.Labels["namespace"]
		callbackData := fmt.Sprintf("%s%d:%s:%s:%s", scaleDeploymentPrefix, incidentID, resourceType, resourceName, namespace)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "↔️ Масштабировать", Data: callbackData}})
	}

	if resourceType == "pod" {
		callbackData := fmt.Sprintf("%s%d:%s:%s", allocateHardwarePrefix, incidentID, resourceType, resourceName)
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "⚙️ Выделить ресурсы", Data: callbackData}})
	}

	keyboard = append(keyboard, []telebot.InlineButton{
		{Text: "⬅️ Назад", Data: showActionsPrefix + strconv.FormatUint(uint64(incidentID), 10)},
		{Text: "🏠 К инциденту", Data: viewIncidentPrefix + strconv.FormatUint(uint64(incidentID), 10)},
	})

	if incident.Status == models.StatusActive {
		keyboard = append(keyboard, []telebot.InlineButton{{Text: "✅ Закрыть инцидент", Data: closeIncidentPrefix + strconv.FormatUint(uint64(incident.ID), 10)}})
	}

	return keyboard
}

func (b *Bot) buildCloseOptionsKeyboard(incidentID uint) [][]telebot.InlineButton {
	idStr := strconv.FormatUint(uint64(incidentID), 10)
	return [][]telebot.InlineButton{
		{
			{Text: "Решен", Data: setStatusPrefix + idStr + ":" + string(models.StatusResolved)},
			{Text: "Отклонен", Data: setStatusPrefix + idStr + ":" + string(models.StatusRejected)},
		},
		{{Text: "⬅️ Назад", Data: viewIncidentPrefix + idStr}},
	}
}

func (b *Bot) authMiddleware() telebot.MiddlewareFunc {
	return func(next telebot.HandlerFunc) telebot.HandlerFunc {
		return func(c telebot.Context) error {
			if c.Sender() == nil {
				return next(c)
			}
			user, err := b.userRepo.FindOrCreateByTelegramID(context.Background(), c.Sender().ID, c.Sender().Username, c.Sender().FirstName, c.Sender().LastName)
			if err != nil {
				log.Printf("Auth middleware error: %v", err)
				return c.Send("Произошла ошибка аутентификации.")
			}
			ctx := context.WithValue(context.Background(), "user", user)
			c.Set("ctx", ctx)
			return next(c)
		}
	}
}

func (b *Bot) formatIncidentMessage(incident *models.Incident) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("*Инцидент: %s*\n", escapeMarkdown(incident.Summary)))
	builder.WriteString(fmt.Sprintf("*Статус:* `%s`\n", incident.Status))
	if severity, ok := incident.Labels["severity"]; ok {
		builder.WriteString(fmt.Sprintf("*Серьезность:* `%s`\n", severity))
	}
	if namespace, ok := incident.Labels["namespace"]; ok {
		builder.WriteString(fmt.Sprintf("*Namespace:* `%s`\n", escapeMarkdown(namespace)))
	}
	builder.WriteString(fmt.Sprintf("*Описание:* %s\n", escapeMarkdown(incident.Description)))
	builder.WriteString(fmt.Sprintf("*Начало:* `%s`\n", incident.StartsAt.Format(time.RFC1123)))

	if len(incident.AuditLog) > 0 {
		builder.WriteString("\n*История действий*\n")
		for _, entry := range incident.AuditLog {
			builder.WriteString(fmt.Sprintf(
				"`%s` \\- *%s* by *%s* \\- *%s*\n",
				entry.Timestamp.Format("15:04:05"),
				escapeMarkdown(entry.Action),
				escapeMarkdown(entry.User.Username),
				escapeMarkdown(entry.Result),
			))
			if entry.Action == "update_status" {
				if reason, ok := entry.Parameters["reason"]; ok && reason != "" {
					builder.WriteString(fmt.Sprintf("  *Причина:* %s\n", escapeMarkdown(reason)))
				}
			}
		}
	}
	return builder.String()
}

func (b *Bot) handleScaleDeployment(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceName := parts[3]
	namespace := parts[4]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)

	req := &models.ActionRequest{
		Action:     string(models.ActionScaleDeployment),
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: map[string]string{
			"deployment": resourceName,
			"namespace":  namespace,
		},
	}

	err := c.Edit("Введите желаемое количество реплик:")
	if err != nil {
		return err
	}

	b.mu.Lock()
	if b.userStates[c.Sender().ID] == nil {
		b.userStates[c.Sender().ID] = &userState{}
	}
	b.userStates[c.Sender().ID].AwaitingReplicaCountFor = &awaitingInputState{
		Request:   req,
		MessageID: c.Message().ID,
		ChatID:    c.Chat().ID,
	}
	b.mu.Unlock()

	return nil
}

func (b *Bot) handleAllocateHardware(c telebot.Context) error {
	parts := strings.Split(c.Data(), ":")
	incidentID, _ := strconv.ParseUint(parts[1], 10, 32)
	resourceName := parts[3]

	user := c.Get("ctx").(context.Context).Value("user").(*models.User)

	req := &models.ActionRequest{
		Action:     string(models.ActionAllocateHardware),
		IncidentID: uint(incidentID),
		UserID:     user.ID,
		Parameters: map[string]string{
			"pod": resourceName,
		},
	}

	err := c.Edit("Введите запрашиваемые ресурсы в формате `cpu=1.5, memory=512Mi`:")
	if err != nil {
		return err
	}

	b.mu.Lock()
	if b.userStates[c.Sender().ID] == nil {
		b.userStates[c.Sender().ID] = &userState{}
	}
	b.userStates[c.Sender().ID].AwaitingHardwareRequestFor = &awaitingInputState{
		Request:   req,
		MessageID: c.Message().ID,
		ChatID:    c.Chat().ID,
	}
	b.mu.Unlock()

	return nil
}

func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]", "(",
		"\\(", ")", "\\)", "~", "\\~", "`", "\\`", ">", "\\>",
		"#", "\\#", "+", "\\+", "-", "\\-", "=", "\\=", "|",
		"\\|", "{", "\\{", "}", "\\}", ".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}
