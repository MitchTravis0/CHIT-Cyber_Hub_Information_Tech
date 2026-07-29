import React from 'react'
import { createRoot } from 'react-dom/client'
import './style.css'
import App from './App'
import { ToastProvider } from './components'
import { ThemeProvider } from './shell/ThemeProvider'

const container = document.getElementById('root')

const root = createRoot(container!)

root.render(
  <React.StrictMode>
    <ThemeProvider>
      <ToastProvider>
        <App />
      </ToastProvider>
    </ThemeProvider>
  </React.StrictMode>,
)
