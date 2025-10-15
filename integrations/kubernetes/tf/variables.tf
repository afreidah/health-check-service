variable "namespace"        { type = string  default = "infra" }
variable "app_name"         { type = string  default = "health-checker" }
variable "image_repo"       { type = string  default = "docker-mirror.service.consul:5000/health-checker" }
variable "image_tag"        { type = string  default = "latest" }
variable "container_port"   { type = number  default = 18081 }
variable "hostport_enabled" { type = bool    default = true }
variable "hostport_port"    { type = number  default = 18081 }
variable "ingress_enabled"  { type = bool    default = true }
variable "ingress_host"     { type = string  default = "health.munchbox" }
variable "service_type"     { type = string  default = "ClusterIP" } # could be NodePort

