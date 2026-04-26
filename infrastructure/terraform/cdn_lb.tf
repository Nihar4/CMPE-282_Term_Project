# ============================================================
# Global HTTPS Load Balancer + Cloud CDN for static frontend
# (the React build lives in the portal_static GCS bucket).
# ============================================================

# Reserve a static global IP
resource "google_compute_global_address" "portal_lb_ip" {
  name = "enterprise-portal-lb-ip"
}

# GCS-backed backend bucket with CDN enabled
resource "google_compute_backend_bucket" "portal_static_bucket_be" {
  name        = "enterprise-portal-static-be"
  bucket_name = google_storage_bucket.portal_static.name
  enable_cdn  = true

  cdn_policy {
    cache_mode                   = "CACHE_ALL_STATIC"
    client_ttl                   = 3600
    default_ttl                  = 3600
    max_ttl                      = 86400
    negative_caching             = true
    serve_while_stale            = 86400
    signed_url_cache_max_age_sec = 7200
  }
}

resource "google_compute_url_map" "portal_url_map" {
  name            = "enterprise-portal-url-map"
  default_service = google_compute_backend_bucket.portal_static_bucket_be.id
}

resource "google_compute_managed_ssl_certificate" "portal_ssl" {
  name = "enterprise-portal-ssl"
  managed {
    domains = [var.domain_name]
  }
}

resource "google_compute_target_https_proxy" "portal_https_proxy" {
  name             = "enterprise-portal-https-proxy"
  url_map          = google_compute_url_map.portal_url_map.id
  ssl_certificates = [google_compute_managed_ssl_certificate.portal_ssl.id]
}

resource "google_compute_global_forwarding_rule" "portal_https" {
  name                  = "enterprise-portal-https-fr"
  ip_address            = google_compute_global_address.portal_lb_ip.address
  port_range            = "443"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  target                = google_compute_target_https_proxy.portal_https_proxy.id
}

# HTTP -> HTTPS redirect
resource "google_compute_url_map" "http_redirect" {
  name = "enterprise-portal-http-redirect"

  default_url_redirect {
    https_redirect         = true
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
    strip_query            = false
  }
}

resource "google_compute_target_http_proxy" "portal_http_proxy" {
  name    = "enterprise-portal-http-proxy"
  url_map = google_compute_url_map.http_redirect.id
}

resource "google_compute_global_forwarding_rule" "portal_http" {
  name                  = "enterprise-portal-http-fr"
  ip_address            = google_compute_global_address.portal_lb_ip.address
  port_range            = "80"
  load_balancing_scheme = "EXTERNAL_MANAGED"
  target                = google_compute_target_http_proxy.portal_http_proxy.id
}

# DNS A-record pointing root to the global LB IP
resource "google_dns_record_set" "portal_root" {
  managed_zone = google_dns_managed_zone.portal_zone.name
  name         = "${var.domain_name}."
  type         = "A"
  ttl          = 300
  rrdatas      = [google_compute_global_address.portal_lb_ip.address]
}
