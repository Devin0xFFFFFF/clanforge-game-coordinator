using System;

namespace Coordinator
{
    [Serializable]
    public class CoordinatorPollResponse
    {
        public const int StatusInQueue = 0;
        public const int StatusJoined = 1;
        public const int StatusMatchmakingCancelled = 2;
        public const int StatusMatchmakingFailed = 3;

        public int Status;
        public string JoinToken;
        public string ServerAddress;
        public int ServerPort;
    }
}
