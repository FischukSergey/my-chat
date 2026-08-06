package push

// Экспорт приватных функций для whitebox-тестирования из пакета push_test.

var (
	BuildPayload                 = buildPayload
	NewWebPushProviderWithClient = newWebPushProviderWithClient
	NormalizeVAPIDSubject        = normalizeVAPIDSubject
)
