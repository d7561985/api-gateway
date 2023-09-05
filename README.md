# API-Gateway

Research project where we could try make declarative API-Gateway for grpc microservices system

## Requirement

* Declarative
* Simple
* Grpc -> Web-Grpc
* Header enrichment: user-ID, session-ID
* Extensible: We need AAA layer inject
* Some endpoint only for authorized users, some - not
* OpenTelemetry start span
* Recapcha
* OPA integration
* Rate limiter

### Consul
https://github.com/hashicorp/learn-consul-service-mesh/tree/main


### Comparison 

|                                     | Amazon API Gateway | Apache APISIX | Traefik | Nginx | Gloo | Envoy |
|-------------------------------------|--------------------|---------------|---------|:------|------|-------|
| Performance                         | 7                  | 10            | 5       |       | 8    | 8-9   |
| Operational Excellence              | Medium             | ?             | ?       |       | ?    | 10    |
| Declarative                         | 5                  | 5             |         |       | 6    | 10    |
| Rate limiter                        | 5                  | 10            | 10      |       | 10   | 10    |
| Header enrichment                   | 5                  | 5             | 5       |       | 5    | 10    |
| AUTH internal                       |                    | 8             | 8       |       |      | 10    |
| AUTH Oauth / Saml                   |                    |               | 10      |       |      | ?10   |
| AUTH OPA integration                |                    |               |         |       |      | ?10   |
| Admission based on authorised users |                    |               |         |       |      | 10    |
| Recapcha                            |                    |               |         |       |      | 10    |
| Observability                       |                    |               |         |       |      | 7     |
| Otel Trace start                    |                    |               |         |       |      | 10    | 


# APPENDIX
* https://github.com/grpc/grpc-web
* https://github.com/fullstorydev/grpcurl
* https://github.com/improbable-eng/grpc-web/tree/master/integration_test
* https://github.com/nfrankel/opentelemetry-tracing
* https://github.com/ktr0731/evans#grpc-web