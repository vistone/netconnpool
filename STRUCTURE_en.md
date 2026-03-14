# Project Structure

English | [中文](STRUCTURE.md)

```
netconnpool/
├── .gemini/                      # Code review and optimization documents
│   ├── code_audit_report.md     # Detailed code audit report
│   ├── audit_summary.md         # Audit summary
│   └── optimization_complete.md # Optimization completion report
│
├── examples/                     # Example code
│   ├── comprehensive_demo.go    # Comprehensive feature demo
│   ├── ipv6_example.go          # IPv6 connection example
│   └── tls/                     # TLS related examples
│       ├── tls_example.go       # TLS client example
│       └── tls_server_example.go # TLS server example
│
├── test/                        # Test files directory
│   ├── README.md                # Test documentation
│   └── pool_race_test.go        # Concurrency safety test
│
├── cleanup.go                   # Connection cleanup manager
├── config.go                    # Configuration structure and validation
├── connection.go                # Connection wrapper and lifecycle management
├── errors.go                    # Error definitions
├── health.go                    # Health check manager
├── ipversion.go                 # IP version detection
├── leak.go                      # Connection leak detector
├── mode.go                      # Pool mode definitions
├── pool.go                      # Core connection pool implementation
├── protocol.go                  # Protocol type detection
├── stats.go                     # Statistics collector
├── udp_utils.go                 # UDP utility functions
├── go.mod                       # Go module definition
├── go.sum                       # Dependency checksums
├── LICENSE                      # BSD-3-Clause license
└── README.md                    # Project documentation

```

## Core Files

### Connection Pool Core
- **pool.go**: Core connection pool implementation with get, put, create logic
- **connection.go**: Connection object wrapper with thread-safe connection info access
- **config.go**: Configuration structure and defaults, supports client/server modes

### Managers
- **health.go**: Health check manager, periodically checks idle connection health
- **cleanup.go**: Cleanup manager, periodically removes expired and unhealthy connections
- **leak.go**: Leak detector, detects connections not returned for long time

### Utilities and Helpers
- **stats.go**: Statistics collector, provides detailed connection pool usage stats
- **protocol.go**: Protocol type detection (TCP/UDP)
- **ipversion.go**: IP version detection (IPv4/IPv6)
- **udp_utils.go**: UDP specific utility functions, e.g., buffer cleanup
- **errors.go**: Error definitions and constants
- **mode.go**: Pool mode definitions (client/server)

## Directory Structure

### examples/
Contains example code for various usage scenarios:
- Basic TCP/UDP connection pool usage
- IPv6 connection support
- TLS encrypted connections
- Lifecycle hooks usage
- Mixed protocol handling

### test/
Contains all test files:
- Concurrency safety tests
- Race condition detection
- Wait queue tests
- Health check tests

Uses external test package (`netconnpool_test`) to ensure only public API is tested.

### .gemini/
Code audit and optimization process documents:
- List of discovered issues
- Fix solutions
- Optimization suggestions
- Test results

## Code Organization Principles

1. **Separation of Concerns**: Each file is responsible for specific functional modules
2. **Clear Naming**: File names directly reflect their functionality
3. **Modular Design**: Managers are independently implemented for easy testing and maintenance
4. **Complete Documentation**: Each directory has corresponding README documentation

## Development Suggestions

- Core logic modifications: Focus mainly on `pool.go` and `connection.go`
- Adding new features: Consider creating new manager files
- Performance optimization: Focus on `stats.go` and lock usage
- Adding tests: Create new test files in `test/` directory
- Adding examples: Create new example programs in `examples/` directory
