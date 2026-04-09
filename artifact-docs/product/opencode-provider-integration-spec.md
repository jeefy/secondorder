# OpenCode Provider Integration Specification

**Date**: April 9, 2026  
**Status**: Draft  
**Owner**: Product Lead  
**Related Issue**: SO-3

## Executive Summary

OpenCode is an open-source AI coding agent and terminal UI that can be integrated as a provider within our existing provider abstraction. It exposes both a command-line interface and HTTP-based API, making it viable for model provider integration and potentially as a code agent backend. This spec outlines the integration requirements, configuration surface, and MVP scope.

## What is OpenCode?

OpenCode is an open-source AI coding agent built for the terminal (similar to Claude Code). It:
- Supports 75+ LLM providers through the Vercel AI SDK
- Exposes an HTTP server API with OpenAPI 3.1 specification
- Manages model configuration through JSON configuration files
- Implements the Model Context Protocol (MCP) for tool integration
- Can be deployed as a standalone headless service

**Key Documentation**: https://opencode.ai/docs/

## 1. APIs and Protocols Exposed

### 1.1 HTTP Server API
OpenCode exposes a headless HTTP server via the `opencode serve` command:
- **Endpoint**: Configurable, default `http://localhost:4096`
- **API Specification**: OpenAPI 3.1 spec available at `/doc` (e.g., `http://localhost:4096/doc`)
- **Protocol**: Standard REST over HTTP

**Key Capabilities**:
- Session management (create, delete, fork, share)
- Message/prompt execution
- File operations and workspace searching
- Provider and authentication management
- Tool configuration and status queries

### 1.2 Model Protocol
OpenCode uses OpenAI-compatible protocol for model interaction:
- **SDK**: Vercel AI SDK (`@ai-sdk/*` packages)
- **Model ID Format**: `provider/model-id` (e.g., `openai/gpt-4`, `opencode/gpt-5.1-codex`)
- **Compatibility**: Any OpenAI-compatible endpoint can be configured

### 1.3 Tool Integration Protocol
- **Protocol**: Model Context Protocol (MCP)
- **Enables**: External tools and services integration with AI assistant
- **Configuration**: Via `opencode.json` MCP servers section

## 2. Integration with Existing Provider Abstraction

### 2.1 Provider Model
OpenCode fits into our provider abstraction as **two potential integration points**:

#### Option A: Model Provider Integration
Treat OpenCode as a backend that proxies requests to multiple underlying providers:
- OpenCode HTTP API becomes a "provider" in our system
- Users configure OpenCode with their preferred models/providers
- Our system calls OpenCode's HTTP API for inference

**Advantages**: 
- Leverages OpenCode's provider management
- Centralized agent configuration
- Tool access through MCP

**Disadvantages**:
- Requires running OpenCode as a service
- Additional operational overhead
- Another abstraction layer

#### Option B: Direct Model Provider Integration (Recommended for MVP)
Integrate directly with OpenCode's supported providers:
- OpenCode identifies available providers and models
- We expose those directly as model choices
- User authenticates with their preferred provider directly

**Advantages**:
- No additional service overhead
- Direct provider integration
- Uses established Vercel AI SDK patterns
- Simpler architecture

**Disadvantages**:
- Must implement each provider's SDK
- No leverage of OpenCode's agent capabilities

### 2.2 Configuration Surface

#### For Option A (OpenCode HTTP API):
```json
{
  "provider": {
    "opencode": {
      "type": "http",
      "name": "OpenCode Agent",
      "baseURL": "{env:OPENCODE_SERVER_URL}",
      "apiKey": "{env:OPENCODE_SERVER_PASSWORD}",
      "capabilities": ["inference", "file_operations", "tool_integration"]
    }
  }
}
```

#### For Option B (Direct Providers):
```json
{
  "provider": {
    "openai": {
      "npm": "@ai-sdk/openai",
      "name": "OpenAI",
      "apiKey": "{env:OPENAI_API_KEY}",
      "models": ["gpt-4", "gpt-3.5-turbo"]
    },
    "anthropic": {
      "npm": "@ai-sdk/anthropic",
      "name": "Anthropic Claude",
      "apiKey": "{env:ANTHROPIC_API_KEY}",
      "models": ["claude-opus", "claude-sonnet"]
    }
  }
}
```

## 3. Configuration Requirements

### 3.1 Environment Variables
OpenCode supports variable substitution in configuration:
```
{env:VARIABLE_NAME}  # Environment variable substitution
{file:path/to/file}  # File content substitution
```

**Required Variables** (depending on selected integration):
- `OPENCODE_SERVER_URL` - For HTTP API integration
- `OPENCODE_SERVER_PASSWORD` - For API authentication
- Provider-specific API keys (e.g., `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`)

### 3.2 Schema Elements
Configuration includes:
- **Provider Section**: Define provider connections and models
- **Models**: Map model IDs to display names and token limits
- **API Key Management**: Credentials stored in `~/.local/share/opencode/auth.json` or via env vars
- **Custom Headers**: Support for authentication headers and provider-specific options
- **Token Limits**: Per-model context window and max output tokens

### 3.3 Model Configuration
```json
{
  "models": {
    "model-id": {
      "name": "Display Name",
      "limit": {
        "context": 200000,
        "output": 65536
      }
    }
  }
}
```

## 4. Limitations and Constraints

### 4.1 Operational
- **Service Requirement**: HTTP API integration requires running OpenCode as a persistent service
- **Network Overhead**: HTTP-based communication adds latency vs. in-process calls
- **Authentication**: Must securely manage `OPENCODE_SERVER_PASSWORD` for remote deployments

### 4.2 Feature
- **Terminal-First Design**: OpenCode is primarily built for terminal usage
- **Model Availability**: Limited to providers supported by Vercel AI SDK (75+ providers)
- **Configuration Format**: Must conform to OpenCode's JSON schema and file-based configuration

### 4.3 Integration
- **MCP Tools**: Tool access requires OpenCode's full setup; may not work through HTTP API
- **Session Management**: Complex workflows may need session state management
- **File Operations**: File-based operations depend on OpenCode's file access policies

## 5. Recommended MVP Scope

### 5.1 Recommendation: Direct Provider Integration (Option B)

**Rationale**:
1. **Simpler Architecture**: No need to run OpenCode as a service
2. **Lower Operational Overhead**: Direct integration with provider SDKs
3. **Better Performance**: No HTTP latency for model inference
4. **Flexibility**: Can support OpenCode's providers incrementally

### 5.2 MVP Phase 1: Core Provider Support
Integrate the most commonly-used providers that OpenCode supports:
- ✅ OpenAI (GPT-4, GPT-3.5)
- ✅ Anthropic (Claude Opus, Sonnet)
- ✅ Google Gemini
- ✅ AWS Bedrock
- ⏳ Groq (phase 2)
- ⏳ Azure OpenAI (phase 2)

### 5.3 MVP Phase 2: Advanced Features
- HTTP API mode for agent capabilities
- MCP tool integration
- Custom endpoint support
- Advanced model configuration

### 5.4 MVP Implementation Tasks

#### Phase 1 (Core Integration)
1. Define provider abstraction in code
2. Implement Vercel AI SDK wrapper
3. Add configuration schema validation
4. Support environment variable substitution
5. Add model discovery and selection UI
6. Implement API key management

#### Phase 2 (OpenCode Agent)
7. Add HTTP API provider type
8. Implement OpenCode service discovery
9. Add session management
10. Implement MCP tool routing

## 6. UX Flow

### 6.1 Provider Selection
1. User opens settings/configuration
2. Selects "Add Provider" or "Configure Provider"
3. Chooses from supported provider list (OpenAI, Anthropic, etc.)
4. Enters API key or authenticates via OAuth
5. System validates credentials
6. Provider added to configuration

### 6.2 Model Selection
1. After provider configuration, user selects model
2. System displays available models for that provider
3. User selects preferred model
4. System saves selection to configuration
5. Model is ready for use in inference

### 6.3 OpenCode Agent Integration (Phase 2)
1. User configures "OpenCode" as provider type
2. Provides OpenCode server URL and password
3. System connects and discovers available providers/models
4. User selects models to expose
5. Agent backend can now use OpenCode for inference

## 7. Technical Integration Points

### 7.1 Configuration Schema
```json
{
  "providers": [
    {
      "id": "provider_id",
      "type": "provider_type",
      "name": "Display Name",
      "credentials": {
        "apiKey": "string"
      },
      "options": {
        "baseURL": "string (optional)",
        "headers": "object (optional)",
        "customOption": "value"
      },
      "models": [
        {
          "id": "model_id",
          "name": "Display Name",
          "contextWindow": 200000,
          "maxOutput": 65536
        }
      ]
    }
  ]
}
```

### 7.2 API Layer
- Provider abstraction layer (abstract methods: authenticate, listModels, invoke)
- Model validation and token limit enforcement
- Error handling and credential management
- Request/response transformation

### 7.3 CLI Integration
- `opencode config add-provider <type>` - Add new provider
- `opencode config list-providers` - List configured providers
- `opencode config test-provider <id>` - Validate provider credentials
- `opencode config set-default-model <provider>/<model>` - Set default

## 8. Security Considerations

### 8.1 Credential Management
- API keys must never be logged or exposed
- Store credentials securely (environment variables or secure storage)
- Support credential rotation
- Mask credentials in logs/UI

### 8.2 Network Security (for HTTP API mode)
- Require HTTPS for remote OpenCode servers
- Enforce strong password for `OPENCODE_SERVER_PASSWORD`
- Implement request signing or OAuth for authenticated access
- Rate limiting on API endpoints

### 8.3 Configuration Security
- Configuration files should not be committed with secrets
- Support .env file pattern
- Validate configuration schema on load
- Warn on insecure configuration (plaintext secrets in config)

## 9. Acceptance Criteria Checklist

- ✅ **API Documentation**: Documented OpenCode APIs, protocols, and integration methods
- ✅ **Provider Abstraction Fit**: Defined how OpenCode fits (Options A & B)
- ✅ **Configuration Surface**: Specified config schema, environment variables, API keys
- ✅ **Limitations & Constraints**: Listed operational, feature, and integration constraints
- ✅ **MVP Recommendation**: Clear scope and phasing for implementation
- ✅ **UX Flow**: Provider and model selection workflows defined
- ✅ **Implementation Tasks**: Phase 1 and 2 tasks identified

## 10. References

- [OpenCode Official Docs](https://opencode.ai/docs/)
- [OpenCode Providers Guide](https://opencode.ai/docs/providers/)
- [OpenCode Configuration](https://opencode.ai/docs/config/)
- [OpenCode Server API](https://opencode.ai/docs/server/)
- [Vercel AI SDK](https://sdk.vercel.ai/)
- [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)

## 11. Next Steps

1. **Stakeholder Review**: Get feedback on Option A vs Option B
2. **Proof of Concept**: Build Phase 1 integration with 1-2 providers
3. **Design Review**: Finalize configuration schema and UX flows
4. **Implementation Planning**: Create detailed technical specification for engineering
