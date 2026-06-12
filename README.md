# MiaoMuse

Local Go + Vue implementation for an AI script creation workspace.

## Run

Backend:

```powershell
cd backend
go run .
```

Frontend:

```powershell
cd frontend
npm install
npm run dev
```

Default frontend URL: `http://localhost:5173`

## Notes

- The app uses mock data and local in-memory API state.
- Paid and quota-consuming operations are simulated with visible point costs.
- The multimodal HTML generation helper is `scripts/generate_html.py`.
- Current retained business scope and table design are documented in `docs/business-logic-current.md`.
