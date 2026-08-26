module Domain exposing (CheckResult, Project, ProjectResults, RedirectHop, Registry, ResultsFile, Status(..), checkResultDecoder, registryDecoder, resultsDecoder, statusLabel, statusRank)

import Dict exposing (Dict)
import Json.Decode as Decode exposing (Decoder)


type alias Registry =
    { schemaVersion : Int
    , projects : List Project
    }


type alias Project =
    { id : String
    , name : String
    , homepage : String
    , manifestUrl : Maybe String
    , imageInfoUrl : Maybe String
    , notes : Maybe String
    }


type Status
    = Pass
    | Advisory
    | Warning
    | Fail
    | Unknown


type alias CheckResult =
    { status : Status
    , summary : String
    , httpStatus : Maybe Int
    , detected : Maybe String
    , contentType : Maybe String
    , corsHeaders : List String
    , location : Maybe String
    , requestAccept : Maybe String
    , responseHeaders : List String
    , redirectChain : List RedirectHop
    }


type alias RedirectHop =
    { url : String
    , httpStatus : Int
    , location : Maybe String
    , responseHeaders : List String
    }


type alias ProjectResults =
    { id : String
    , name : String
    , checked : Bool
    , checkedAt : Maybe String
    , checks : Dict String CheckResult
    }


type alias ResultsFile =
    { generatedAt : Maybe String
    , projects : List ProjectResults
    }


registryDecoder : Decoder Registry
registryDecoder =
    Decode.map2 Registry
        (Decode.field "schemaVersion" Decode.int)
        (Decode.field "projects" (Decode.list projectDecoder))


projectDecoder : Decoder Project
projectDecoder =
    Decode.map6 Project
        (Decode.field "id" Decode.string)
        (Decode.field "name" Decode.string)
        (Decode.field "homepage" Decode.string)
        (Decode.maybe (Decode.field "manifestUrl" Decode.string))
        (Decode.maybe (Decode.field "imageInfoUrl" Decode.string))
        (Decode.maybe (Decode.field "notes" Decode.string))


resultsDecoder : Decoder ResultsFile
resultsDecoder =
    Decode.map2 ResultsFile
        (Decode.field "generatedAt" (Decode.nullable Decode.string))
        (Decode.field "projects" (Decode.list projectResultsDecoder))


projectResultsDecoder : Decoder ProjectResults
projectResultsDecoder =
    Decode.map5 ProjectResults
        (Decode.field "id" Decode.string)
        (Decode.field "name" Decode.string)
        (Decode.oneOf [ Decode.field "checked" Decode.bool, Decode.succeed True ])
        (Decode.maybe (Decode.field "checkedAt" Decode.string))
        (Decode.field "checks" (Decode.dict checkResultDecoder))


checkResultDecoder : Decoder CheckResult
checkResultDecoder =
    Decode.map2 (\build redirectChain -> build redirectChain)
        (Decode.map2 (\build responseHeaders -> build responseHeaders)
            (Decode.map8 CheckResult
                (Decode.field "status" statusDecoder)
                (Decode.field "summary" Decode.string)
                (Decode.maybe (Decode.field "httpStatus" Decode.int))
                (Decode.maybe (Decode.field "detected" Decode.string))
                (Decode.maybe (Decode.field "contentType" Decode.string))
                (Decode.oneOf [ Decode.field "corsHeaders" (Decode.list Decode.string), Decode.succeed [] ])
                (Decode.maybe (Decode.field "location" Decode.string))
                (Decode.maybe (Decode.field "requestAccept" Decode.string))
            )
            (Decode.oneOf [ Decode.field "responseHeaders" (Decode.list Decode.string), Decode.succeed [] ])
        )
        (Decode.oneOf [ Decode.field "redirectChain" (Decode.list redirectHopDecoder), Decode.succeed [] ])


redirectHopDecoder : Decoder RedirectHop
redirectHopDecoder =
    Decode.map4 RedirectHop
        (Decode.field "url" Decode.string)
        (Decode.field "httpStatus" Decode.int)
        (Decode.maybe (Decode.field "location" Decode.string))
        (Decode.oneOf [ Decode.field "responseHeaders" (Decode.list Decode.string), Decode.succeed [] ])


statusDecoder : Decoder Status
statusDecoder =
    Decode.string
        |> Decode.andThen
            (\value ->
                case value of
                    "pass" ->
                        Decode.succeed Pass

                    "advisory" ->
                        Decode.succeed Advisory

                    "warning" ->
                        Decode.succeed Warning

                    "fail" ->
                        Decode.succeed Fail

                    "unknown" ->
                        Decode.succeed Unknown

                    _ ->
                        Decode.fail ("Unknown status: " ++ value)
            )


statusLabel : Status -> String
statusLabel status =
    case status of
        Pass ->
            "Pass"

        Advisory ->
            "Advisory"

        Warning ->
            "Warning"

        Fail ->
            "Fail"

        Unknown ->
            "Not tested"


statusRank : Status -> Int
statusRank status =
    case status of
        Fail ->
            4

        Warning ->
            3

        Advisory ->
            2

        Pass ->
            1

        Unknown ->
            0
