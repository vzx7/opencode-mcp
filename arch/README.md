# Architecture Documentation

MCP-сервер для AI-ассистированного аудита архитектуры проектов.  
Поддерживает Go (полная реализация), Python и TypeScript (stub-реализации).

## Навигация

| Документ | Содержание |
|---|---|
| [01-requirements.md](01-requirements.md) | Функциональные и нефункциональные требования |
| [02-system-context.md](02-system-context.md) | Акторы, внешние системы, границы |
| [03-architecture-overview.md](03-architecture-overview.md) | Слои, компоненты, потоки данных |
| [04-compliance-spec.md](04-compliance-spec.md) | Схема архитектурных правил и правила этого проекта |
| [adr/001-mcp-protocol.md](adr/001-mcp-protocol.md) | Выбор протокола MCP / JSON-RPC 2.0 |
| [adr/002-provider-pattern.md](adr/002-provider-pattern.md) | Абстракция LLM-провайдеров |
| [adr/003-static-analysis.md](adr/003-static-analysis.md) | Статический анализ и алгоритмы обнаружения нарушений |
| [adr/004-prompt-strategy.md](adr/004-prompt-strategy.md) | Локализованные JSON-промты и LLM-only scoring |
| [adr/005-compliance-rules-schema.md](adr/005-compliance-rules-schema.md) | Формат правил для compliance check |
| [adr/006-multi-language.md](adr/006-multi-language.md) | Плагин-архитектура для поддержки нескольких языков |

## Статус

Документация описывает целевую архитектуру системы.  
ADR фиксируют принятые решения с обоснованием и рассмотренными альтернативами.
