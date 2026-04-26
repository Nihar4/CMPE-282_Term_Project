# Serverless Squad — CMPE-282 Term Project

| Member                                  | GitHub                          | Role(s)                                                          |
| --------------------------------------- | ------------------------------- | ---------------------------------------------------------------- |
| **Aditya Govind Shahari**               | [@adityashahari1][a]            | Frontend & UX, AI integration, demo lead                         |
| **Mohsen Minai**                        | _pending — share GitHub handle_ | Backend services (Go), security, CORS / JWT / IAM                |
| **Nihar Dharmeshkumar Patel**           | [@Nihar4][n]                    | Cloud architecture, GCP / Terraform / Kubernetes, CI/CD          |
| **Tamizh Selvan Manivannan**            | _pending — share GitHub handle_ | Data engineering, parser-service (Python), analytics             |

[a]: https://github.com/adityashahari1
[n]: https://github.com/Nihar4

> **Group**: Serverless Squad — 4 students.
> **Course**: CMPE-282 (Cloud Technologies) — Spring 2026.
> **GitHub**: https://github.com/Nihar4/CMPE-282_Term_Project

---

## How we worked

- **Branching**: trunk-based; every change went through a PR with at least one
  reviewer from outside the author's domain (e.g., a frontend PR was reviewed
  by a backend dev and vice-versa).
- **Communication**: weekly sync, Slack channel for async, shared Notion for
  the design doc.
- **Tasks**: tracked as GitHub Issues with labels (`area/backend`,
  `area/frontend`, `area/infra`, `area/cicd`, `area/docs`).

## Contribution areas (high-level)

| Area                                     | Lead                              |
| ---------------------------------------- | --------------------------------- |
| React 18 + MUI portal, AI Chat, Dashboards | Aditya Govind Shahari            |
| Go microservices, JWT, RBAC, gateway     | Mohsen Minai                      |
| GCP infra, Terraform, GKE, Jenkins, Okta | Nihar Dharmeshkumar Patel         |
| Python parser service, data pipelines, analytics | Tamizh Selvan Manivannan  |

## Adding the team as GitHub collaborators

Once each member shares their GitHub username, run:

```bash
gh api -X PUT repos/Nihar4/CMPE-282_Term_Project/collaborators/<username> \
  -f permission=push
```

(or invite via the GitHub UI: *Settings → Collaborators → Add people*).
