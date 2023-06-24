# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

service {
  name = "api"
  id = "api-v1"
  address = "10.5.0.4"
  port = 9092

  tags = ["v1"]
  meta = {
    version = "1"
  }

  check = {
    name = "Check api health 9092"
    #http= "http://10.5.0.4:9092/health"
    #method= "GET"
    tcp = "10.5.0.4:9092"
    interval= "10s"
    timeout=  "1s"
  }

  connect {
    sidecar_service {
      port = 20000

      check {
        name     = "Connect Envoy Sidecar"
        tcp      = "10.5.0.4:20000"
        interval = "10s"
      }
    }
  }
}