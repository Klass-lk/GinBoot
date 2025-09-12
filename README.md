# Ginboot Framework

A lightweight and powerful Go web framework built on top of Gin, designed for building scalable web applications with MongoDB integration and AWS Lambda support.

## Setup

### Prerequisites
- Go 1.21 or later
- MongoDB (for local development)
- AWS SAM CLI (for deployment)
- AWS credentials configured

### Installation

1. Install the Ginboot CLI tool:
```bash
go install github.com/klass-lk/ginboot-cli@latest
```

2. Create a new project:
```bash
# Create a new project
ginboot new myproject

# Navigate to project directory
cd myproject

# Initialize dependencies
go mod tidy
```

3. Run locally:
```bash
go run main.go
```
Your API will be available at `http://localhost:8080/api/v1`

### Build and Deploy

To deploy your application to AWS Lambda:

```bash
# Build the project for AWS Lambda
ginboot build

# Deploy to AWS
ginboot deploy
```

On first deployment, you'll be prompted for:
- Stack name (defaults to project name)
- AWS Region
- S3 bucket configuration

These settings will be saved in `ginboot-app.yml` for future deployments.

## Features

- **Database Operations**: Built-in multi-database support (MongoDB, SQL, DynamoDB) through a generic repository interface, enabling common CRUD operations with minimal code.
- **API Request Handling**: Simplified API request and authentication context extraction.
- **Error Handling**: Easily define and manage business errors.
- **Password Encoding**: Inbuilt password hashing and matching utility for secure authentication.
- **CORS Configuration**: Flexible CORS setup with both default and custom configurations.

## Installation

To install GinBoot, add it to your project:

```bash
go get github.com/klass-lk/ginboot
```

## Documentation

For more detailed information on Ginboot's features and usage, refer to the following documentation:

*   [Server Configuration](docs/server.md)
*   [Routing](docs/routing.md)
*   [Authentication](docs/authentication.md)
*   [Database Support](docs/database.md)
*   [Deployment to AWS Lambda using SAM](docs/deployment.md)

## Contributing
Contributions are welcome! Please read our contributing guidelines for more details.

## License
This project is licensed under the MIT License. See the LICENSE file for details.
