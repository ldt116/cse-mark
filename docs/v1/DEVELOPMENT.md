# Development Guide

## Tools

- **Dependency Injection:**
  - Use [Google Wire](https://github.com/google/wire) for dependency injection code generation.
  - To generate DI code for the each service, such as
    ```sh
    cd cmd/marksync
    wire
    ```

## Architecture

This project follows the Clean Architecture principles:

- **Domain Layer**: Contains core business logic and domain entities (see `internal/domain`).
- **Use Cases Layer**: Application-specific business rules (see `internal/usecases`).
- **Interface Adapters**: Controllers, gateways, presenters (see `internal/delivery`).
- **Infrastructure Layer**: Frameworks, drivers, DB clients (see `internal/infra`).

### Directory Structure

- `cmd/` - Entrypoints for different services (fetcher, http, tele)
- `internal/configs/` - Configuration loading and environment variables
- `internal/domain/` - Domain models, repositories, and business rules
  - `course/` - Course domain models and repository interfaces
  - `mark/` - Mark domain models and repository interfaces
  - `user/` - User domain models and repository interfaces
  - `teleuser/` - Telegram user domain models
  - `downloader/` - Downloader domain interfaces
- `internal/usecases/` - Application use cases
  - `iam/` - Identity and Access Management use cases
  - `marksync/` - Mark synchronization use cases
  - `markimport/` - Mark import use cases
  - `coursequery/` - Course query use cases
- `internal/infra/` - Infrastructure code (MongoDB, logging)
- `internal/delivery/` - Delivery mechanisms (Telegram)

### Development Workflow

1. Define or update domain models, repositories, and business rules in the `internal/domain` package.
2. Implement application-specific use cases in the `internal/usecases` package, using interfaces from the domain layer.
3. Add or update infrastructure code in the `internal/infra` package, implementing interfaces defined in the domain layer.
4. Add delivery mechanisms in the `internal/delivery` package, wiring up services and use cases to external interfaces.
5. Use dependency injection (see Tools section) to compose the application in the `cmd/` entrypoints.

## Coding Standards

- Use Go modules for dependency management.
- Write unit tests for business logic in the domain and usecase layers.
- Use [zerolog](https://github.com/rs/zerolog) for logging.
- Use environment variables for configuration (see `internal/configs`).

## Running the Project

- Copy `.env.example` to `.env` and update values as needed.
- Use Docker Compose for local development:
  ```sh
  docker-compose up --build
  ```

## Contributing

- Follow Clean Architecture guidelines for new features.
- Write clear, concise commit messages.
- Document public APIs and exported functions.
