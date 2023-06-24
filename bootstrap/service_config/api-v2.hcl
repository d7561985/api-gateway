# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

service {
  name = "api"
  id = "api-v2"
  address = "10.5.0.5"
  port = 9092

  tags = ["v2"]
  meta = {
    version = "2"
  }

  check {
    name = "Check api health 9092"
    tcp = "10.5.0.5:9092"
    #http= "http://10.5.0.5:9092/health"
    #method= "GET"
    interval= "10s"
    timeout=  "1s"
  }

  connect {
    sidecar_service {
      port = 20000

      check {
        name     = "Connect Envoy Sidecar"
        tcp      = "10.5.0.5:20000"
        interval = "10s"
      }
    }
  }
}