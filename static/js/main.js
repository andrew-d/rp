/*
 * This file is the main JavaScript file for the Offline Wiki app.
 *
 * Unlike the Service Worker, this file should use only common JavaScript
 * features and APIs that are broadly supported.
 */

// Register service worker when the page loads.
var swRegistration;
if ('serviceWorker' in navigator) {
  window.addEventListener('load', function() {
    navigator.serviceWorker.register('/js/service-worker.js', {
      scope: '/', // the entire site
    })
      .then(function(registration) {
        console.log('ServiceWorker registration successful');
        swRegistration = registration;
      })
      .catch(function(err) {
        console.log('ServiceWorker registration failed: ', err);
      });
  });
}

// Sample wiki content
const wikiContent = {
    home: `
        <h1>Welcome to Offline Wiki</h1>
        <p>This is a demo wiki that works offline! Here's a sample image:</p>
        <img src="/images/cat.jpeg" alt="Sample image">
        <p>Try turning off your internet connection and refreshing the page.</p>
    `,
    about: `
        <h1>About</h1>
        <p>This wiki demonstrates offline capabilities using Service Workers.</p>
        <img src="/images/cat2.jpeg" alt="Another sample image">
    `,
    help: `
        <h1>Help</h1>
        <p>To test offline functionality:</p>
        <ol>
            <li>Visit a few pages to cache them</li>
            <li>Turn off your internet connection</li>
            <li>Try navigating the wiki</li>
        </ol>
    `,
    cache: `
        <h1>Cache Information</h1>
        <p>Below are all the currently cached pages:</p>
        <div id="cache-info" class="cache-list">
            Click the button to refresh cache info.
        </div>
        <button onclick="refreshCacheInfo()" class="refresh-button">Refresh Cache Info</button>
    `
};

 // Function to get cache info from Service Worker
async function refreshCacheInfo() {
  console.log('Refreshing cache info...');

  const cacheInfoDiv = document.getElementById('cache-info');
  cacheInfoDiv.innerHTML = 'Loading cache information...';

  if (!navigator.serviceWorker.controller) {
    cacheInfoDiv.innerHTML = 'Service Worker not yet active. Please refresh the page.';
    return;
  }

  try {
    const messageChannel = new MessageChannel();
    messageChannel.port1.onmessage = event => {
      const urls = event.data;
      console.log('Cache contents:', urls);

      if (urls.length === 0) {
        cacheInfoDiv.innerHTML = 'No pages currently cached.';
        return;
      }

      cacheInfoDiv.innerHTML = '<ul>' +
        urls.map(url => `<li>${url}</li>`).join('') +
        '</ul>';
    };

    navigator.serviceWorker.controller.postMessage({
      type: 'getCacheContents'
    }, [messageChannel.port2]);
  } catch (error) {
    cacheInfoDiv.innerHTML = 'Error fetching cache information: ' + error.message;
  }
}

// Handle navigation
document.getElementById('nav-links').addEventListener('click', function(e) {
  if (e.target.tagName === 'A') {
    e.preventDefault();
    const page = e.target.dataset.page;
    loadPage(page);
  }
});

// Load page content
function loadPage(page) {
  const content = wikiContent[page] || 'Page not found';
  document.getElementById('content').innerHTML = content;
}

// This function updates the online status in the UI.
function updateOnlineStatus() {
  const status = document.getElementById('status');
  status.style.display = 'block';
  status.textContent = navigator.onLine ? 'Online' : 'Offline';
  status.style.backgroundColor = navigator.onLine ? '#28a745' : '#dc3545';
  setTimeout(function() {
    status.style.display = 'none';
  }, 2000);
}

window.addEventListener('online', updateOnlineStatus);
window.addEventListener('offline', updateOnlineStatus);

// Load home page by default
loadPage('home');
