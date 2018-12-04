namespace Coordinator
{
    public struct MatchFoundInfo
    {
        public readonly string ServerAddress;
        public readonly int ServerPort;
        public readonly string JoinGameToken;

        public MatchFoundInfo(string serverAddress, int serverPort, string joinGameToken)
        {
            ServerAddress = serverAddress;
            ServerPort = serverPort;
            JoinGameToken = joinGameToken;
        }
    }
}
