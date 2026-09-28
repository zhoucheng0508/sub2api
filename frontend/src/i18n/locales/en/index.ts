// CUSTOM(VOTE-AI-RIKKAHUB): messages remain owned by the extension.
import rikkaHubMessages from '@/custom/vote-ai/rikkahub/messages'
import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'

export default {
  rikkaHub: rikkaHubMessages.en,
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  admin,
  ...misc,
}
