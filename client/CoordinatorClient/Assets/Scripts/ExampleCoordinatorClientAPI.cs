using System;
using System.Collections;
using UnityEngine;
using UnityEngine.Networking;

namespace Coordinator
{
    public class ExampleCoordinatorClientAPI : ICoordinatorClientAPI
    {
        private readonly string _address;
        private readonly float _pollRate;
        private readonly int _maxRetriesUntilFail;
        private readonly int _maxErrorsUntilCancel;

        private readonly string _enqueueURL;
        private readonly string _dequeueURL;
        private readonly string _pollURL;

        private bool _requestInProgress;
        private bool _searching;
        private string _queryToken;

        public ExampleCoordinatorClientAPI(string address, int pollRate, int maxRetriesUntilFail = 3, int maxErrorsUntilCancel = 5)
        {
            _address = address;
            _pollRate = pollRate;
            _maxRetriesUntilFail = maxRetriesUntilFail;
            _maxErrorsUntilCancel = maxErrorsUntilCancel;

            _enqueueURL = string.Format("https://{0}/enqueue", _address);
            _dequeueURL = string.Format("https://{0}/dequeue", _address);
            _pollURL = string.Format("https://{0}/poll", _address);
        }

        public bool StartSearch(ulong userID, string authToken, string region, Action onSearchFailed, out IEnumerator routine)
        {
            if (_requestInProgress || _searching)
            {
                routine = null;
                return false;
            }

            routine = _TryStart(userID, authToken, region, onSearchFailed);
            return true;
        }

        public bool CancelSearch(Action onCancel, out IEnumerator routine)
        {
            if (_requestInProgress || !_searching)
            {
                routine = null;
                return false;
            }

            routine = _TryCancel(onCancel);
            return true;
        }

        private IEnumerator _TryStart(ulong userID, string authToken, string region, Action onSearchFailed)
        {
            _requestInProgress = true;

            var uriBuilder = new UriBuilder(_enqueueURL)
            {
                Query = string.Format("UserID={0}&AuthToken={1}&Region={2}", userID, authToken, region)
            };

            int tries = 0;

            while(tries < _maxRetriesUntilFail)
            {
                using (var request = UnityWebRequest.Get(uriBuilder.Uri))
                {
                    yield return request.SendWebRequest();

                    if (request.isNetworkError)
                    {
                        Debug.Log(string.Format("[COORDINATOR] Network Error: {0}", request.error));
                    }
                    else if (request.isHttpError)
                    {
                        Debug.Log(string.Format("[COORDINATOR] HTTP Error: {0} ({1})", request.responseCode, request.error));
                    }
                    else
                    {
                        _queryToken = request.downloadHandler.text;
                        _searching = true;

                        break;
                    }
                }

                tries++;

                yield return new WaitForSeconds(_pollRate * tries);
            }

            if(tries >= _maxRetriesUntilFail) { onSearchFailed(); }

            _requestInProgress = false;
        }

        private IEnumerator _TryCancel(Action onCancel)
        {
            _requestInProgress = true;

            _searching = false;

            Uri requestURI = new UriBuilder(_dequeueURL) { Query = string.Format("QueryToken={0}", _queryToken) }.Uri;

            using (var request = UnityWebRequest.Get(requestURI))
            {
                yield return request.SendWebRequest();

                if (request.isNetworkError)
                {
                    Debug.Log(string.Format("[COORDINATOR] Network Error: {0}", request.error));
                }
                else if (request.isHttpError)
                {
                    Debug.Log(string.Format("[COORDINATOR] HTTP Error: {0} ({1})", request.responseCode, request.error));
                }
                else if (request.responseCode == 200)
                {
                    _queryToken = null;
                }
            }

            onCancel();

            _requestInProgress = false;
        }

        public IEnumerator _Poll(Action<MatchFoundInfo> onMatchFound, Action onSearchFailed)
        {
            Uri requestURI = new UriBuilder(_pollURL) { Query = string.Format("QueryToken={0}", _queryToken) }.Uri;
            int errors = 0;

            while (true)
            {
                yield return new WaitForSeconds(_pollRate);

                if(!_searching) { break; }

                using (var request = UnityWebRequest.Get(requestURI))
                {
                    yield return request.SendWebRequest();

                    if (request.isNetworkError)
                    {
                        Debug.Log(string.Format("[COORDINATOR] Network Error: {0}", request.error));
                        errors++;
                    }
                    else if (request.isHttpError)
                    {
                        Debug.Log(string.Format("[COORDINATOR] HTTP Error: {0} ({1})", request.responseCode, request.error));
                        errors++;
                    }
                    else if (request.responseCode == 200)
                    {
                        string content = request.downloadHandler.text;

                        var response = JsonUtility.FromJson<CoordinatorPollResponse>(content);

                        if (response.Status == CoordinatorPollResponse.StatusJoined)
                        {
                            Debug.Log(string.Format("[COORDINATOR] Joined Match: {0}:{1} (Token={2})", response.ServerAddress, response.ServerPort, response.JoinToken));
                            onMatchFound(new MatchFoundInfo(response.ServerAddress, response.ServerPort, response.JoinToken));
                            break;
                        }
                        else if (response.Status == CoordinatorPollResponse.StatusMatchmakingFailed)
                        {
                            Debug.Log("[COORDINATOR] Matchmaking failed.");

                            IEnumerator routine;
                            CancelSearch(onSearchFailed, out routine);
                            yield return routine;

                            break;
                        }
                        else if (response.Status == CoordinatorPollResponse.StatusMatchmakingCancelled)
                        {
                            Debug.Log("[COORDINATOR] Matchmaking cancelled.");

                            IEnumerator routine;
                            CancelSearch(onSearchFailed, out routine);
                            yield return routine;

                            break;
                        }

                        errors = 0;
                    }
                }

                if(errors > _maxErrorsUntilCancel)
                {
                    IEnumerator routine;
                    CancelSearch(onSearchFailed, out routine);
                    yield return routine;

                    break;
                }
            }
        }
    }
}
