# Certificate Maintenance for maintainer-d

maintainer-d uses a manually issued Let’s Encrypt certificate stored in the `maintainerd-tls` secret. 

Repeat these steps before the cert expires (every ~60–90 days):

1. **Request a new certificate via certbot**
   ```bash
   sudo certbot certonly \
     --manual \
     --preferred-challenges dns \
     --key-type rsa \
     -d github-events.cncf.io \
     --email <EMAIL_ADDRESS> \
     --agree-tos
   ```
   Certbot prints a `_acme-challenge.github-events.cncf.io` TXT record. Ask the DNSimple admin to create it (TTL 60). Press Enter once DNS propagates.

2. **Load the cert into Kubernetes**
   ```bash
   kubectl create secret tls maintainerd-tls \
     --cert=/etc/letsencrypt/live/github-events.cncf.io/fullchain.pem \
     --key=/etc/letsencrypt/live/github-events.cncf.io/privkey.pem \
     -n maintainerd \
     --dry-run=client -o yaml | kubectl apply -f -
   ```

3. **Recreate the Service so OCI reloads the cert**
   ```bash
   kubectl delete svc maintainerd -n maintainerd
   kubectl apply -f deploy/manifests/service.yaml
   kubectl get svc maintainerd -n maintainerd --watch
   ```
   Wait until `EXTERNAL-IP` shows `170.9.21.206` again.

4. **Verify**
   ```bash
   kubectl describe svc maintainerd -n maintainerd
   curl -vk https://github-events.cncf.io/healthz
   openssl s_client -connect github-events.cncf.io:443 -servername github-events.cncf.io -tls1_2
   ```

Keep `/etc/letsencrypt` backed up or document the certbot host. If you ever automate DNS updates, you can replace the manual step with cert-manager and remove the monthly coordination with DNSimple.
