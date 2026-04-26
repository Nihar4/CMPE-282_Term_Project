import React from 'react';
import ReactDOM from 'react-dom/client';
import { Auth0Provider } from '@auth0/auth0-react';
import { BrowserRouter } from 'react-router-dom';
import App from './App';

function Auth0ProviderWithNavigate({ children }: { children: React.ReactNode }) {
  return (
    <Auth0Provider
      domain={process.env.REACT_APP_AUTH0_DOMAIN || 'dev-xbnsordr5elttyug.us.auth0.com'}
      clientId={process.env.REACT_APP_AUTH0_CLIENT_ID || 'x8pFzWFtyYCBXrr6U2NkxABroQqqMvxM'}
      cacheLocation="localstorage"
      useRefreshTokens
      useRefreshTokensFallback
      authorizationParams={{
        redirect_uri: window.location.origin,
        audience: process.env.REACT_APP_AUTH0_AUDIENCE,
      }}
      onRedirectCallback={(appState) => {
        const target = (appState?.returnTo as string) || window.location.pathname;
        window.history.replaceState({}, document.title, target);
      }}
    >
      {children}
    </Auth0Provider>
  );
}

const root = ReactDOM.createRoot(document.getElementById('root') as HTMLElement);

root.render(
  <React.StrictMode>
    <BrowserRouter>
      <Auth0ProviderWithNavigate>
        <App />
      </Auth0ProviderWithNavigate>
    </BrowserRouter>
  </React.StrictMode>
);
