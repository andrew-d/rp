/*
 * This Service Worker caches pages as they are visited, and serves them from
 * the cache when offline.
 *
 * Note that Service Workers are only supported on HTTPS sites, except on localhost.
 *
 * Service Workers are not supported in all browsers, but as far as I can tell
 * supporting Service Workers also implies supporting arrow functions,
 * const/let, and similar modern-ish JavaScript features.
 */

const CACHE_NAME = 'wiki-v1';
const DEBUG = true;

function debug(message) {
  if (DEBUG) {
    console.log(message);
  }
}

// This event is triggered when the Service Worker is installed.
self.addEventListener('install', event => {
  const BASE_URL = self.location.pathname.replace('sw.js', '');
  const AUTO_CACHE = [
    '/',
    '/js/service-worker.js',
  ];

  // From MDN: 
  //    Note: Because install/activate events could take a while to complete,
  //    the service worker spec provides a waitUntil() method. Once it is
  //    called on install or activate events with a promise, functional events
  //    such as fetch and push will wait until the promise is successfully
  //    resolved.
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => {
        // Only cache the main page initially
        debug('Caching base pages: ' + AUTO_CACHE.join(', '));
        return cache.addAll(AUTO_CACHE);
      })
      .then(self.skipWaiting())
  );
});

// This event is triggered when the Service Worker is activated; after
// installation, and when the page is refreshed.
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(cacheNames => {
        return cacheNames.filter(nn => nn !== CACHE_NAME);
      })
      .then(cacheNames => {
        return Promise.all(
          cacheNames.map(cacheName => {
            return caches.delete(cacheName);
          })
        );
      })
      .then(() => {
        // Per MDN: https://developer.mozilla.org/en-US/docs/Web/API/Service_Worker_API
        //
        //    If there is an existing service worker available, the new version
        //    is installed in the background, but not yet activated — at this
        //    point it is called the worker in waiting. It is only activated
        //    when there are no longer any pages loaded that are still using
        //    the old service worker. As soon as there are no more pages to be
        //    loaded, the new service worker activates (becoming the active
        //    worker). Activation can happen sooner using
        //    ServiceWorkerGlobalScope.skipWaiting() and existing pages can be
        //    claimed by the active worker using Clients.claim().
        return self.clients.claim();
      })
  );
});

// This event is triggered to intercept requests made by the page.
// 
// See: https://developer.mozilla.org/en-US/docs/Web/API/FetchEvent
self.addEventListener('fetch', event => {
  if (!event.request.url.startsWith(self.location.origin) || event.request.method !== 'GET') {
    // External request, or POST, ignore
    event.respondWith(fetch(event.request));
    return;
  }

  event.respondWith(
    // Try network first
    fetch(event.request)
      .then(response => {
        // Only cache successful responses
        if (!response || response.status !== 200 || response.type !== 'basic') {
          return response;
        }

        // Cache the new response
        debug('Caching response: ' + event.request.url);
        const responseToCache = response.clone();
        caches.open(CACHE_NAME)
          .then(cache => {
            cache.put(event.request, responseToCache);
          });

        return response;
      })
      .catch(() => {
        // If network fails, try cache
        return caches.match(event.request)
          .then(response => {
            if (response) {
              debug('Returning cached response: ' + response.url);
              return response;
            }
            // If not in cache, return a fallback
            debug('Returning fallback response');

            // TODO: get offline page from cache?
            return new Response('Content not available offline', {
              headers: { 'Content-Type': 'text/plain' }
            });
          });
      })
  );
});

// Handle messages from the client; see MDN:
// https://developer.mozilla.org/en-US/docs/Web/API/ServiceWorkerGlobalScope/message_event
self.addEventListener('message', event => {
  console.log('Message received:', event.data);

  switch (event.data.type) {
    case 'getCacheContents':
      handleGetCacheContents(event);

    default:
      debug('Unknown message type: ' + event.data.type);
      break;
  }
});

function handleGetCacheContents(event) {
  const port = event.ports[0];

  caches.open(CACHE_NAME)
    .then(cache => cache.keys())
    .then(requests => requests.map(request => request.url))
    .then(urls => {
      // Send the URLs back to the client
      port.postMessage(urls);
    })
    .catch(error => {
      port.postMessage([`Error: ${error.message}`]);
    });
}
